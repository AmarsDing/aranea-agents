package knowledge

import (
	"context"
	"strings"
	"time"

	trpcdoc "trpc.group/trpc-go/trpc-agent-go/knowledge/document"
	trpcreranker "trpc.group/trpc-go/trpc-agent-go/knowledge/reranker"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// KnowledgeRerankerAdapter wraps a trpc-agent-go knowledge reranker
// to implement the biz.Reranker interface used by memory L2/L3 recall.
// It delegates scoring to the knowledge reranker's Rerank API,
// falling back to bigram Jaccard on error or empty results.
type KnowledgeRerankerAdapter struct {
	reranker trpcreranker.Reranker
	lg       loggateway.Logger
}

// Compile-time check: KnowledgeRerankerAdapter implements biz.Reranker.
var _ biz.Reranker = (*KnowledgeRerankerAdapter)(nil)

// NewKnowledgeRerankerAdapter creates a new adapter wrapping the given knowledge reranker.
func NewKnowledgeRerankerAdapter(r trpcreranker.Reranker, lg loggateway.Logger) *KnowledgeRerankerAdapter {
	return &KnowledgeRerankerAdapter{reranker: r, lg: lg}
}

func (a *KnowledgeRerankerAdapter) Score(query, passage string) float64 {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	results, err := a.reranker.Rerank(ctx,
		&trpcreranker.Query{Text: query},
		[]*trpcreranker.Result{
			{Document: &trpcdoc.Document{ID: "0", Content: passage}},
		},
	)
	if err != nil {
		a.lg.Warn("knowledge reranker error, falling back to jaccard",
			loggateway.StepID("knowledge.memory_reranker"), loggateway.Err(err))
		return fallbackJaccard(query, passage)
	}
	if len(results) == 0 {
		return fallbackJaccard(query, passage)
	}
	return results[0].Score
}

// fallbackJaccard computes bigram Jaccard similarity with normalized inputs,
// used when the knowledge reranker is unavailable or errors out.
// Kept as a bigram-only path (no unigram fallback) so CE-error scoring
// stays identical to the former data-layer adapter.
func fallbackJaccard(query, passage string) float64 {
	return bigramJaccard(
		strings.ToLower(strings.TrimSpace(query)),
		strings.ToLower(strings.TrimSpace(passage)),
	)
}

func bigramJaccard(a, b string) float64 {
	if a == "" || b == "" {
		return 0
	}
	ga := wordBigrams(a)
	gb := wordBigrams(b)
	if len(ga) == 0 || len(gb) == 0 {
		return 0
	}
	inter := 0
	for k := range ga {
		if _, ok := gb[k]; ok {
			inter++
		}
	}
	union := len(ga) + len(gb) - inter
	if union == 0 {
		return 0
	}
	return float64(inter) / float64(union)
}

func wordBigrams(text string) map[string]struct{} {
	tokens := strings.Fields(text)
	out := make(map[string]struct{})
	if len(tokens) == 1 {
		out[tokens[0]] = struct{}{}
		return out
	}
	for i := 0; i < len(tokens)-1; i++ {
		out[tokens[i]+" "+tokens[i+1]] = struct{}{}
	}
	return out
}
