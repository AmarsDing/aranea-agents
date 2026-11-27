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

// FilteredVectorStore is an optional VectorStore extension: similarity search
// restricted to rows whose meta[key] equals value for every filter entry.
// Shared tables (vector_embeddings holds every user's facts) need it — a
// global top-K followed by an SQL scope filter returns almost nothing for one
// user once many users share the table. Callers type-assert and fall back to
// the unfiltered Search when a store does not implement it.
// Stability:evolving
type FilteredVectorStore interface {
	SearchFiltered(ctx context.Context, embedding []float64, topK int, minScore float64, filter map[string]string) ([]VectorHit, error)
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
