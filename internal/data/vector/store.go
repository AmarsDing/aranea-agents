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

// FactVectorStore extends VectorStore with fact-specific operations.
// Stability:evolving
type FactVectorStore interface {
	VectorStore
	// UpsertFact inserts or replaces the vector row for a memory fact.
	UpsertFact(ctx context.Context, id string, agentID string, userID string, content string, embedding []float64) error
	// SearchByAgent returns the top-K most similar facts filtered by agent and user.
	SearchByAgent(ctx context.Context, agentID string, userID string, embedding []float64, topK int, minScore float64) ([]VectorHit, error)
}
