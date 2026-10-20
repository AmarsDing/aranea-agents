package knowledge

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
	"trpc.group/trpc-go/trpc-agent-go/knowledge/reranker"
	"trpc.group/trpc-go/trpc-agent-go/knowledge/reranker/cohere"
	"trpc.group/trpc-go/trpc-agent-go/knowledge/reranker/infinity"
	"trpc.group/trpc-go/trpc-agent-go/knowledge/reranker/topk"
)

// KnowledgeRerankSettingsView is the knowledge-package view of persisted
// reranker settings (U2-5). It mirrors biz.KnowledgeRerankSettings field for
// field but stays a separate type so the builder contract below is owned by
// this package and callers adapt at the wiring edge — the two packages
// exchange state only through view/interface types.
type KnowledgeRerankSettingsView struct {
	Provider  string
	Model     string
	BaseURL   string
	APIKey    string
	Enabled   bool
	UpdatedAt string
	HasAPIKey bool
}

// KnowledgeRerankSettingsViewFromBiz adapts the biz settings to the view.
func KnowledgeRerankSettingsViewFromBiz(s biz.KnowledgeRerankSettings) KnowledgeRerankSettingsView {
	return KnowledgeRerankSettingsView{
		Provider:  s.Provider,
		Model:     s.Model,
		BaseURL:   s.BaseURL,
		APIKey:    s.APIKey,
		Enabled:   s.Enabled,
		UpdatedAt: s.UpdatedAt,
		HasAPIKey: s.HasAPIKey,
	}
}

// BuildRerankerWithSettings resolves the reranker by priority (U2-5):
//
//  1. persisted settings — when the getter supplies Enabled settings with a
//     valid provider, the framework reranker is constructed with semantics
//     aligned to NewRerankerFromEnv (reranker_factory.go; alignment points
//     annotated in buildRerankerFromView);
//  2. existing env/factory logic — NewRerankerFromEnv (KRATOS_KNOWLEDGE_*
//     family);
//  3. bigram-Jaccard degrade — biz.NewCrossEncoderReranker.
//
// Framework rerankers are wrapped in KnowledgeRerankerAdapter so the result
// satisfies biz.Reranker; the adapter itself degrades to Jaccard on backend
// errors at score time. Any read/construction error degrades one tier and is
// reported through logf exactly once (logf may be nil). The getter is invoked
// with context.Background(): reranker construction happens off the request
// path, mirroring NewMemoryReranker.
func BuildRerankerWithSettings(getter func(context.Context) (KnowledgeRerankSettingsView, error), logf func(string, ...any)) biz.Reranker {
	if getter != nil {
		s, err := getter(context.Background())
		switch {
		case err != nil:
			logRerankSettings(logf, "knowledge reranker: settings read failed, falling back to env factory: %v", err)
		case !s.Enabled:
			// Disabled settings are not an error: skip to the env tier silently.
		case !knowledgeRerankProviderConstructible(s.Provider):
			logRerankSettings(logf, "knowledge reranker: settings provider %q not constructible, falling back to env factory", s.Provider)
		default:
			if r, err := buildRerankerFromView(s); err != nil {
				logRerankSettings(logf, "knowledge reranker: settings construction failed, falling back to env factory: %v", err)
			} else {
				return NewKnowledgeRerankerAdapter(r, loggateway.NewNoop())
			}
		}
	}
	r, err := NewRerankerFromEnv()
	if err != nil {
		logRerankSettings(logf, "knowledge reranker: env factory failed, degrading to jaccard: %v", err)
		return biz.NewCrossEncoderReranker()
	}
	if r == nil {
		return biz.NewCrossEncoderReranker()
	}
	return NewKnowledgeRerankerAdapter(r, loggateway.NewNoop())
}

// knowledgeRerankProviderConstructible reports whether the provider id can be
// built from settings. The authoritative set is biz.KnowledgeRerankProvider*;
// literals are repeated here to stay visually aligned with the switch in
// NewRerankerFromEnv.
func knowledgeRerankProviderConstructible(provider string) bool {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "topk", "cohere", "infinity":
		return true
	}
	return false
}

// buildRerankerFromView constructs the framework reranker from persisted
// settings. Construction semantics align with NewRerankerFromEnv
// (reranker_factory.go) — alignment points:
//   - provider ids and normalization (lower/trim) are identical; unknown
//     providers return the same Internal error shape;
//   - topk: K still comes from KRATOS_KNOWLEDGE_RERANK_TOP_K via the same
//     rerankEnvInt helper (settings carry no K field; 0 = all results);
//   - cohere: the API key option is always passed; model/endpoint options
//     only when non-empty; TopN honored from the same env var when > 0;
//   - infinity: endpoint/api-key/model options only when non-empty —
//     infinity.New errors on an empty endpoint, which degrades the caller one
//     tier per BuildRerankerWithSettings; TopN honored as above.
func buildRerankerFromView(s KnowledgeRerankSettingsView) (reranker.Reranker, error) {
	topK := rerankEnvInt("KRATOS_KNOWLEDGE_RERANK_TOP_K", 0)
	switch strings.ToLower(strings.TrimSpace(s.Provider)) {
	case "topk":
		return topk.New(topk.WithK(topK)), nil
	case "cohere":
		opts := []cohere.Option{
			cohere.WithAPIKey(strings.TrimSpace(s.APIKey)),
		}
		if m := strings.TrimSpace(s.Model); m != "" {
			opts = append(opts, cohere.WithModel(m))
		}
		if ep := strings.TrimSpace(s.BaseURL); ep != "" {
			opts = append(opts, cohere.WithEndpoint(ep))
		}
		if topK > 0 {
			opts = append(opts, cohere.WithTopN(topK))
		}
		return cohere.New(opts...)
	case "infinity":
		opts := []infinity.Option{}
		if ep := strings.TrimSpace(s.BaseURL); ep != "" {
			opts = append(opts, infinity.WithEndpoint(ep))
		}
		if key := strings.TrimSpace(s.APIKey); key != "" {
			opts = append(opts, infinity.WithAPIKey(key))
		}
		if m := strings.TrimSpace(s.Model); m != "" {
			opts = append(opts, infinity.WithModel(m))
		}
		if topK > 0 {
			opts = append(opts, infinity.WithTopN(topK))
		}
		return infinity.New(opts...)
	default:
		return nil, apierror.Internal(apierror.DomainKnowledge, "knowledge reranker: unknown settings provider %q", s.Provider)
	}
}

// logRerankSettings reports a degradation exactly once through logf; nil-safe.
func logRerankSettings(logf func(string, ...any), format string, args ...any) {
	if logf != nil {
		logf(format, args...)
	}
}
