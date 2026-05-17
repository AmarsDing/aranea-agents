// Package knowledge provides vector retrieval for knowledge base search.
package knowledge

import (
	"context"
	"fmt"

	"aranea-agents/internal/biz"
)

// Retriever orchestrates embedding + search for a collection.
type Retriever struct {
	embedder *Embedder
	repo     biz.KnowledgeRepo
}

// NewRetriever creates a Retriever bound to the given embedder and repo.
func NewRetriever(embedder *Embedder, repo biz.KnowledgeRepo) *Retriever {
	return &Retriever{embedder: embedder, repo: repo}
}

// Search embeds the query and returns the top-k most relevant chunks.
func (r *Retriever) Search(ctx context.Context, q biz.KnowledgeSearchQuery) ([]biz.KnowledgeChunk, error) {
	if r.embedder == nil {
		return nil, fmt.Errorf("retriever: embedder is nil")
	}
	if r.repo == nil {
		return nil, fmt.Errorf("retriever: repo is nil")
	}
	vec, err := r.embedder.Embed(ctx, q.Query)
	if err != nil {
		return nil, fmt.Errorf("retriever embed: %w", err)
	}
	return r.repo.SearchChunks(ctx, q, vec)
}
