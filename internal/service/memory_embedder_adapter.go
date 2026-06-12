package service

import (
	"context"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/knowledge"
	"aranea-agents/pkg/apierror"
)

type MemoryEmbeddingAdapter struct {
	embedder knowledge.QueryEmbedder
}

var _ biz.EmbeddingService = (*MemoryEmbeddingAdapter)(nil)

func NewMemoryEmbeddingAdapter(embedder knowledge.QueryEmbedder) *MemoryEmbeddingAdapter {
	return &MemoryEmbeddingAdapter{embedder: embedder}
}

func (a *MemoryEmbeddingAdapter) Embed(ctx context.Context, text string) ([]float32, error) {
	if a == nil || a.embedder == nil {
		return nil, apierror.Unavailable("MEMORY", "embedding service not available")
	}
	return a.embedder.EmbedSingle(ctx, text)
}
