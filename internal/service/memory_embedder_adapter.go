package service

import (
	"context"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/knowledge"
)

type MemoryEmbeddingAdapter struct {
	embedder knowledge.QueryEmbedder
}

var _ biz.EmbeddingService = (*MemoryEmbeddingAdapter)(nil)

func NewMemoryEmbeddingAdapter(embedder knowledge.QueryEmbedder) *MemoryEmbeddingAdapter {
	return &MemoryEmbeddingAdapter{embedder: embedder}
}

func (a *MemoryEmbeddingAdapter) Embed(ctx context.Context, text string) ([]float32, error) {
	return a.embedder.EmbedSingle(ctx, text)
}
