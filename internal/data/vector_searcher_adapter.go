package data

import (
	"context"

	"aranea-agents/internal/data/vector"
)

// vectorSearcherAdapter adapts vector.VectorStore to the local VectorSearcher interface.
type vectorSearcherAdapter struct {
	store vector.VectorStore
}

var _ VectorSearcher = (*vectorSearcherAdapter)(nil)

func newVectorSearcherAdapter(store vector.VectorStore) *vectorSearcherAdapter {
	return &vectorSearcherAdapter{store: store}
}

func (a *vectorSearcherAdapter) Search(ctx context.Context, embedding []float64, topK int, minScore float64) ([]VectorSearchHit, error) {
	hits, err := a.store.Search(ctx, embedding, topK, minScore)
	if err != nil {
		return nil, err
	}
	out := make([]VectorSearchHit, len(hits))
	for i, h := range hits {
		out[i] = VectorSearchHit{ID: h.ID, Score: h.Score}
	}
	return out, nil
}
