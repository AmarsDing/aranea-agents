package vector

import "context"

// VectorHit represents a single search result from a vector store.
type VectorHit struct {
	ID    string
	Score float64
	Meta  map[string]string
}

// VectorStore abstracts vector storage and similarity search.
type VectorStore interface {
	// Upsert inserts or updates a vector embedding for the given ID.
	Upsert(ctx context.Context, id string, embedding []float64, meta map[string]string) error

	// Search returns the top-K most similar vectors to the query embedding.
	Search(ctx context.Context, embedding []float64, topK int, minScore float64) ([]VectorHit, error)

	// Delete removes a vector by ID.
	Delete(ctx context.Context, id string) error
}
