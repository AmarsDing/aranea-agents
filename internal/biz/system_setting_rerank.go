package biz

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"aranea-agents/pkg/apierror"
)

// ── Knowledge reranker settings (U2-5) ───────────────────────────────────────
//
// Persisted platform defaults for the knowledge reranker, mirroring the
// KnowledgeEmbed family on SystemSettingUsecase (see system_setting.go).
// Storage contract: the concrete system-setting repo persists the config as
// single-line JSON on the system_settings singleton row under the column/key
// KnowledgeRerankConfigKey (same storage family as the knowledge_embed_* /
// refine_llm_* raw-SQL columns; ent schema regeneration is blocked, so new
// settings ride raw SQL columns).

// Knowledge reranker provider identifiers constructible from settings.
const (
	KnowledgeRerankProviderTopk     = "topk"
	KnowledgeRerankProviderCohere   = "cohere"
	KnowledgeRerankProviderInfinity = "infinity"
)

// KnowledgeRerankConfigKey is the system_settings singleton-row key holding
// the one-line reranker config JSON (key style mirrors knowledge_embed_*).
const KnowledgeRerankConfigKey = "knowledge_rerank_config"

// KnowledgeRerankSettings is the persisted platform default for the knowledge
// reranker (U2-5). APIKey is plaintext only in stored/internal form; the
// usecase getters return it redacted (APIKey empty, HasAPIKey marking
// presence) except GetKnowledgeRerankPlaintext, which is for internal factory
// construction only.
type KnowledgeRerankSettings struct {
	Provider  string
	Model     string
	BaseURL   string
	APIKey    string
	Enabled   bool
	UpdatedAt string
	HasAPIKey bool
}

// knowledgeRerankConfigJSON is the persisted wire format under
// KnowledgeRerankConfigKey. HasAPIKey is derived on read and never stored.
type knowledgeRerankConfigJSON struct {
	Provider  string `json:"provider,omitempty"`
	Model     string `json:"model,omitempty"`
	BaseURL   string `json:"base_url,omitempty"`
	APIKey    string `json:"api_key,omitempty"`
	Enabled   bool   `json:"enabled,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

// MarshalKnowledgeRerankConfig serializes stored settings as single-line JSON
// (encoding/json emits no newlines). Repository implementations persist the
// result verbatim under KnowledgeRerankConfigKey.
func MarshalKnowledgeRerankConfig(s KnowledgeRerankSettings) ([]byte, error) {
	out, err := json.Marshal(knowledgeRerankConfigJSON{
		Provider:  s.Provider,
		Model:     s.Model,
		BaseURL:   s.BaseURL,
		APIKey:    s.APIKey,
		Enabled:   s.Enabled,
		UpdatedAt: s.UpdatedAt,
	})
	if err != nil {
		return nil, apierror.Internal("SYSTEM_SETTING", "marshal knowledge rerank config: %v", err)
	}
	return out, nil
}

// UnmarshalKnowledgeRerankConfig parses the persisted single-line JSON,
// deriving HasAPIKey from api_key presence. Empty input yields zero settings.
func UnmarshalKnowledgeRerankConfig(raw []byte) (KnowledgeRerankSettings, error) {
	if len(strings.TrimSpace(string(raw))) == 0 {
		return KnowledgeRerankSettings{}, nil
	}
	var j knowledgeRerankConfigJSON
	if err := json.Unmarshal(raw, &j); err != nil {
		return KnowledgeRerankSettings{}, apierror.Internal("SYSTEM_SETTING", "unmarshal knowledge rerank config: %v", err)
	}
	return KnowledgeRerankSettings{
		Provider:  j.Provider,
		Model:     j.Model,
		BaseURL:   j.BaseURL,
		APIKey:    j.APIKey,
		Enabled:   j.Enabled,
		UpdatedAt: j.UpdatedAt,
		HasAPIKey: strings.TrimSpace(j.APIKey) != "",
	}, nil
}

// NormalizeKnowledgeRerankProvider lower-cases and trims a provider id.
func NormalizeKnowledgeRerankProvider(p string) string {
	return strings.ToLower(strings.TrimSpace(p))
}

// KnowledgeRerankProviderValid reports whether the (normalized) provider is
// one the knowledge factory can construct from settings.
func KnowledgeRerankProviderValid(p string) bool {
	switch NormalizeKnowledgeRerankProvider(p) {
	case KnowledgeRerankProviderTopk, KnowledgeRerankProviderCohere, KnowledgeRerankProviderInfinity:
		return true
	}
	return false
}

// ApplyKnowledgeRerankPatch merges an update onto current settings, mirroring
// ApplyEmbedPatch semantics: provider is normalized and replaced when
// supplied, base_url/model are always overwritten with their trimmed patch
// value (proto3 zero-value form posts), and an empty patch APIKey preserves
// the stored key.
func ApplyKnowledgeRerankPatch(cur, patch KnowledgeRerankSettings) KnowledgeRerankSettings {
	out := cur
	if p := NormalizeKnowledgeRerankProvider(patch.Provider); p != "" {
		out.Provider = p
	}
	out.BaseURL = strings.TrimRight(strings.TrimSpace(patch.BaseURL), "/")
	out.Model = strings.TrimSpace(patch.Model)
	out.Enabled = patch.Enabled
	if k := strings.TrimSpace(patch.APIKey); k != "" {
		out.APIKey = k
		out.HasAPIKey = true
	}
	return out
}

// redactKnowledgeRerank returns the API-safe view: APIKey always empty,
// HasAPIKey marking stored-key presence.
func redactKnowledgeRerank(s KnowledgeRerankSettings) KnowledgeRerankSettings {
	s.HasAPIKey = s.HasAPIKey || strings.TrimSpace(s.APIKey) != ""
	s.APIKey = ""
	return s
}

// KnowledgeRerankSettingRepo is the persistence port for knowledge reranker
// settings. The concrete system-setting repo satisfies it by storing the
// config as single-line JSON on the system_settings singleton row under
// KnowledgeRerankConfigKey (see Marshal/UnmarshalKnowledgeRerankConfig).
//
// Contract: Get returns the stored plaintext settings (API key included) and
// a zero KnowledgeRerankSettings without error when no config exists yet;
// Update persists the merged value verbatim.
//
// Stability:evolving
type KnowledgeRerankSettingRepo interface {
	GetKnowledgeRerank(ctx context.Context) (KnowledgeRerankSettings, error)
	UpdateKnowledgeRerank(ctx context.Context, merged KnowledgeRerankSettings) (KnowledgeRerankSettings, error)
}

// knowledgeRerankRepo resolves the reranker-settings port off the usecase's
// system-setting repo without widening SystemSettingRepo (kept on the same
// receiver via a capability assertion so the data layer can land the storage
// half independently).
func (u *SystemSettingUsecase) knowledgeRerankRepo() (KnowledgeRerankSettingRepo, error) {
	rr, ok := u.repo.(KnowledgeRerankSettingRepo)
	if !ok {
		return nil, apierror.Internal("SYSTEM_SETTING", "knowledge rerank settings repository not available")
	}
	return rr, nil
}

// GetKnowledgeRerank returns stored knowledge reranker settings with the API
// key redacted (APIKey always empty; HasAPIKey marks presence) — safe for
// admin API responses.
func (u *SystemSettingUsecase) GetKnowledgeRerank(ctx context.Context) (KnowledgeRerankSettings, error) {
	rr, err := u.knowledgeRerankRepo()
	if err != nil {
		return KnowledgeRerankSettings{}, err
	}
	s, err := rr.GetKnowledgeRerank(ctx)
	if err != nil {
		return KnowledgeRerankSettings{}, err
	}
	return redactKnowledgeRerank(s), nil
}

// GetKnowledgeRerankPlaintext returns stored settings including the plaintext
// API key — internal reranker factory construction only, mirroring the
// GetKnowledgeEmbed plaintext contract. Never expose via admin APIs.
func (u *SystemSettingUsecase) GetKnowledgeRerankPlaintext(ctx context.Context) (KnowledgeRerankSettings, error) {
	rr, err := u.knowledgeRerankRepo()
	if err != nil {
		return KnowledgeRerankSettings{}, err
	}
	return rr.GetKnowledgeRerank(ctx)
}

// UpdateKnowledgeRerank merges the patch onto stored settings and persists
// it: an empty patch APIKey preserves the stored key; the provider is
// normalized and must be one of topk/cohere/infinity when set; enabling
// without a provider is rejected. The merged row is stamped UpdatedAt (UTC
// RFC3339) and the redacted view is returned.
func (u *SystemSettingUsecase) UpdateKnowledgeRerank(ctx context.Context, in KnowledgeRerankSettings) (KnowledgeRerankSettings, error) {
	rr, err := u.knowledgeRerankRepo()
	if err != nil {
		return KnowledgeRerankSettings{}, err
	}
	cur, err := rr.GetKnowledgeRerank(ctx)
	if err != nil {
		return KnowledgeRerankSettings{}, err
	}
	merged := ApplyKnowledgeRerankPatch(cur, in)
	if merged.Provider != "" && !KnowledgeRerankProviderValid(merged.Provider) {
		return KnowledgeRerankSettings{}, apierror.BadRequest("SYSTEM_SETTING", "knowledge_rerank_provider must be one of topk, cohere, infinity")
	}
	if merged.Enabled && merged.Provider == "" {
		return KnowledgeRerankSettings{}, apierror.BadRequest("SYSTEM_SETTING", "knowledge_rerank_provider is required when enabled")
	}
	merged.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	out, err := rr.UpdateKnowledgeRerank(ctx, merged)
	if err != nil {
		return KnowledgeRerankSettings{}, err
	}
	return redactKnowledgeRerank(out), nil
}
