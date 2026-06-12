//go:build pgvector

package vector

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/pgvector/pgvector-go"
)

// PgVectorFactStore implements FactVectorStore using a dimension-specific
// agent_memory_<dim> table with agent_id / user_id columns.
type PgVectorFactStore struct {
	db        *sql.DB
	tableName string
	dim       int
}

// NewPgVectorFactStore creates a PgVectorFactStore bound to the
// agent_memory_<dim> table. The caller must ensure the table already exists
// (e.g. via the legacy pgvector.EnsureDimensionTable or a future migration).
func NewPgVectorFactStore(db *sql.DB, dim int) (*PgVectorFactStore, error) {
	if db == nil {
		return nil, sql.ErrConnDone
	}
	if dim <= 0 {
		dim = 1536
	}
	return &PgVectorFactStore{
		db:        db,
		tableName: fmt.Sprintf("agent_memory_%d", dim),
		dim:       dim,
	}, nil
}

// Upsert inserts or replaces a vector row (base VectorStore).
func (s *PgVectorFactStore) Upsert(ctx context.Context, id string, embedding []float64, meta map[string]string) error {
	agentID, _ := meta["agent_id"]
	userID, _ := meta["user_id"]
	content, _ := meta["content"]
	return s.UpsertFact(ctx, id, agentID, userID, content, embedding)
}

// Search returns the top-K most similar vectors (base VectorStore).
// Without agent/user filtering it falls back to unfiltered search.
func (s *PgVectorFactStore) Search(ctx context.Context, embedding []float64, topK int, minScore float64) ([]VectorHit, error) {
	if topK <= 0 {
		topK = 10
	}
	f32 := float64To32(embedding)
	vec := pgvector.NewVector(f32)
	q := fmt.Sprintf(`SELECT id, 1 - (embedding <=> $1) AS score, '' AS content
		FROM %s
		WHERE 1 - (embedding <=> $1) >= $3
		ORDER BY embedding <=> $1
		LIMIT $2`, s.tableName)
	rows, err := s.db.QueryContext(ctx, q, vec, topK, minScore)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanFactHits(rows)
}

// Delete removes a vector by ID (base VectorStore).
func (s *PgVectorFactStore) Delete(ctx context.Context, id string) error {
	q := fmt.Sprintf(`DELETE FROM %s WHERE id = $1`, s.tableName)
	_, err := s.db.ExecContext(ctx, q, id)
	return err
}

// UpsertFact inserts or replaces the vector row for a memory fact.
// It first deletes any existing row with the same fact_id prefix for the
// agent/user pair, then inserts the new row.
func (s *PgVectorFactStore) UpsertFact(ctx context.Context, id string, agentID string, userID string, content string, embedding []float64) error {
	f32 := float64To32(embedding)
	if len(f32) != s.dim {
		return fmt.Errorf("embedding dimension mismatch: got %d want %d", len(f32), s.dim)
	}
	prefix := FactVectorContentPrefix(id)
	delQ := fmt.Sprintf(`DELETE FROM %s WHERE agent_id = $1 AND user_id = $2 AND content LIKE $3`, s.tableName)
	if _, err := s.db.ExecContext(ctx, delQ, agentID, userID, prefix+"%"); err != nil {
		return err
	}
	vec := pgvector.NewVector(f32)
	insQ := fmt.Sprintf(`INSERT INTO %s (agent_id, user_id, content, embedding) VALUES ($1, $2, $3, $4)`, s.tableName)
	_, err := s.db.ExecContext(ctx, insQ, agentID, userID, factVectorContent(id, content), vec)
	return err
}

// SearchByAgent returns the top-K most similar facts filtered by agent and user.
func (s *PgVectorFactStore) SearchByAgent(ctx context.Context, agentID string, userID string, embedding []float64, topK int, minScore float64) ([]VectorHit, error) {
	if topK <= 0 {
		topK = 10
	}
	f32 := float64To32(embedding)
	if len(f32) != s.dim {
		return nil, fmt.Errorf("embedding dimension mismatch: got %d want %d", len(f32), s.dim)
	}
	vec := pgvector.NewVector(f32)
	q := fmt.Sprintf(`SELECT id, 1 - (embedding <=> $3) AS score, content
		FROM %s
		WHERE agent_id = $1 AND user_id = $2
		  AND 1 - (embedding <=> $3) >= $5
		ORDER BY embedding <=> $3
		LIMIT $4`, s.tableName)
	rows, err := s.db.QueryContext(ctx, q, agentID, userID, vec, topK, minScore)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanFactHits(rows)
}

func scanFactHits(rows *sql.Rows) ([]VectorHit, error) {
	var hits []VectorHit
	for rows.Next() {
		var id int64
		var score float64
		var content string
		if err := rows.Scan(&id, &score, &content); err != nil {
			return nil, err
		}
		meta := map[string]string{}
		if content != "" {
			meta["content"] = content
		}
		hits = append(hits, VectorHit{
			ID:    fmt.Sprintf("%d", id),
			Score: score,
			Meta:  meta,
		})
	}
	return hits, rows.Err()
}

// ensure PgVectorFactStore implements FactVectorStore at compile time.
var _ FactVectorStore = (*PgVectorFactStore)(nil)

// TableNameForDimension returns the partitioned table name for embeddings of fixed width dim.
func TableNameForDimension(dim int) string {
	return fmt.Sprintf("agent_memory_%d", dim)
}

// EnsureDimensionTable creates ONE table for embeddings of dimension dim (idempotent).
func EnsureDimensionTable(ctx context.Context, db *sql.DB, dim int) error {
	if dim <= 0 || dim > 16000 {
		return fmt.Errorf("invalid vector dimension %d", dim)
	}
	tbl := TableNameForDimension(dim)
	idx := fmt.Sprintf("idx_%s_agent_uid", tbl)
	ddl := fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %[1]s (
  id BIGSERIAL PRIMARY KEY,
  agent_id TEXT NOT NULL,
  user_id TEXT NOT NULL DEFAULT '',
  content TEXT NOT NULL,
  embedding vector(%[2]d) NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS %[3]s ON %[1]s (agent_id, user_id);
`, tbl, dim, idx)
	if _, err := db.ExecContext(ctx, ddl); err != nil {
		return fmt.Errorf("ddl %s: %w", tbl, err)
	}
	return nil
}

// EnsureExtension creates the pgvector extension if missing (once per DB).
func EnsureExtension(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, `CREATE EXTENSION IF NOT EXISTS vector`); err != nil {
		return fmt.Errorf("create extension vector: %w", err)
	}
	return nil
}

// EnsureSchema applies extension plus the default singleton table agent_memory_<defaultDim>.
func EnsureSchema(ctx context.Context, db *sql.DB, dim int) error {
	if err := EnsureExtension(ctx, db); err != nil {
		return err
	}
	return EnsureDimensionTable(ctx, db, dim)
}

// Dim returns the configured embedding dimension.
func (s *PgVectorFactStore) Dim() int {
	if s == nil || s.dim <= 0 {
		return 1536
	}
	return s.dim
}

// Table returns the table name.
func (s *PgVectorFactStore) Table() string {
	if s == nil || s.tableName == "" {
		return TableNameForDimension(s.Dim())
	}
	return s.tableName
}

// SearchNearest returns rows matching the legacy pgvector.Store.SearchNearest signature.
// This is a transitional helper for the old memoryRepo; new code should use SearchByAgent.
func (s *PgVectorFactStore) SearchNearest(ctx context.Context, agentID, userID string, queryEmbedding []float64, limit int) ([]VectorHit, error) {
	return s.SearchByAgent(ctx, agentID, userID, queryEmbedding, limit, 0)
}

// Insert inserts a raw memory row (agent_id, user_id, content, embedding).
// Transitional helper for the old memoryRepo.
func (s *PgVectorFactStore) Insert(ctx context.Context, agentID, userID, content string, embedding []float64) error {
	f32 := float64To32(embedding)
	if len(f32) != s.dim {
		return fmt.Errorf("embedding dimension mismatch: got %d want %d", len(f32), s.dim)
	}
	vec := pgvector.NewVector(f32)
	q := fmt.Sprintf(`INSERT INTO %s (agent_id, user_id, content, embedding) VALUES ($1, $2, $3, $4)`, s.tableName)
	_, err := s.db.ExecContext(ctx, q, agentID, userID, content, vec)
	return err
}

// UpsertFactVector replaces the pgvector read-index row for a memory_facts id.
// Transitional helper matching the old pgvector.Store.UpsertFactVector signature.
func (s *PgVectorFactStore) UpsertFactVector(ctx context.Context, agentID, userID, factID, statement string, embedding []float64) error {
	return s.UpsertFact(ctx, factID, agentID, userID, statement, embedding)
}

// Row mirrors the legacy pgvector.Row for transitional compatibility.
type Row struct {
	ID        int64
	AgentID   string
	UserID    string
	Content   string
	Distance  float64
}

// SearchNearestRows returns legacy Row objects for the old memoryRepo.
func (s *PgVectorFactStore) SearchNearestRows(ctx context.Context, agentID, userID string, queryEmbedding []float64, limit int) ([]Row, error) {
	if limit <= 0 {
		limit = 10
	}
	f32 := float64To32(queryEmbedding)
	if len(f32) != s.dim {
		return nil, fmt.Errorf("embedding dimension mismatch: got %d want %d", len(f32), s.dim)
	}
	vec := pgvector.NewVector(f32)
	q := fmt.Sprintf(`
SELECT id, agent_id, user_id, content, embedding <=> $3::vector AS distance
FROM %s
WHERE agent_id = $1 AND user_id = $2
ORDER BY embedding <=> $3::vector
LIMIT $4`, s.tableName)
	rows, err := s.db.QueryContext(ctx, q, agentID, userID, vec, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Row
	for rows.Next() {
		var r Row
		if err := rows.Scan(&r.ID, &r.AgentID, &r.UserID, &r.Content, &r.Distance); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// String returns a debug representation.
func (s *PgVectorFactStore) String() string {
	if s == nil {
		return "PgVectorFactStore(nil)"
	}
	return fmt.Sprintf("PgVectorFactStore(table=%s, dim=%d)", s.tableName, s.dim)
}

// IsPgvector reports whether the build includes pgvector support.
func IsPgvector() bool { return true }

// TrimSpace is re-exported for internal use.
func trimSpace(s string) string { return strings.TrimSpace(s) }
