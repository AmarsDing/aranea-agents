//go:build !pgvector

package vector

import (
	"context"
	"database/sql"
	"fmt"
)

// PgVectorFactStore is a stub that returns errors when pgvector build tag is not enabled.
type PgVectorFactStore struct{}

// Compile-time interface check.
var _ FactVectorStore = (*PgVectorFactStore)(nil)

// NewPgVectorFactStore returns an error when pgvector build tag is not enabled.
func NewPgVectorFactStore(db *sql.DB, dim int) (*PgVectorFactStore, error) {
	return nil, fmt.Errorf("pgvector fact store requires build tag 'pgvector': %w", sql.ErrConnDone)
}

// EnsureExtension returns an error when pgvector build tag is not enabled.
func EnsureExtension(ctx context.Context, db *sql.DB) error {
	return fmt.Errorf("pgvector extension requires build tag 'pgvector'")
}

// EnsureDimensionTable returns an error when pgvector build tag is not enabled.
func EnsureDimensionTable(ctx context.Context, db *sql.DB, dim int) error {
	return fmt.Errorf("pgvector dimension table requires build tag 'pgvector'")
}

// EnsureSchema returns an error when pgvector build tag is not enabled.
func EnsureSchema(ctx context.Context, db *sql.DB, dim int) error {
	return fmt.Errorf("pgvector schema requires build tag 'pgvector'")
}

// TableNameForDimension returns the table name even without pgvector support
// (useful for display/logging).
func TableNameForDimension(dim int) string {
	return fmt.Sprintf("agent_memory_%d", dim)
}

// IsPgvector reports whether the build includes pgvector support.
func IsPgvector() bool { return false }

// FactVectorStore interface stubs.

func (s *PgVectorFactStore) Upsert(ctx context.Context, id string, embedding []float64, meta map[string]string) error {
	return errPgvectorNotAvailable()
}

func (s *PgVectorFactStore) Search(ctx context.Context, embedding []float64, topK int, minScore float64) ([]VectorHit, error) {
	return nil, errPgvectorNotAvailable()
}

func (s *PgVectorFactStore) Delete(ctx context.Context, id string) error {
	return errPgvectorNotAvailable()
}

func (s *PgVectorFactStore) UpsertFact(ctx context.Context, id string, agentID string, userID string, content string, embedding []float64) error {
	return errPgvectorNotAvailable()
}

func (s *PgVectorFactStore) SearchByAgent(ctx context.Context, agentID string, userID string, embedding []float64, topK int, minScore float64) ([]VectorHit, error) {
	return nil, errPgvectorNotAvailable()
}

func errPgvectorNotAvailable() error {
	return fmt.Errorf("pgvector not available: build with -tags pgvector")
}
