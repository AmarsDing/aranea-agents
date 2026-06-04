//go:build pgvector

package vector

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"aranea-agents/pkg/loggateway"

	"github.com/pgvector/pgvector-go"
)

// PgVectorStore implements VectorStore using the pgvector PostgreSQL extension.
type PgVectorStore struct {
	db        *sql.DB
	tableName string
	dim       int
	lg        loggateway.Logger
}

// NewPgVectorStore creates a new PgVectorStore and ensures the table exists.
func NewPgVectorStore(db *sql.DB, tableName string, dim int, lg loggateway.Logger) (VectorStore, error) {
	if db == nil {
		return nil, sql.ErrConnDone
	}
	tableName = strings.TrimSpace(tableName)
	if tableName == "" {
		tableName = "vector_embeddings"
	}
	if dim <= 0 {
		dim = 1536
	}
	s := &PgVectorStore{db: db, tableName: tableName, dim: dim, lg: lg}
	if err := s.ensureTable(context.Background()); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *PgVectorStore) ensureTable(ctx context.Context) error {
	ddl := fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
		id TEXT PRIMARY KEY,
		embedding vector(%d) NOT NULL,
		meta JSONB NOT NULL DEFAULT '{}'
	)`, s.tableName, s.dim)
	_, err := s.db.ExecContext(ctx, ddl)
	return err
}

// Upsert inserts or updates a vector embedding for the given ID.
func (s *PgVectorStore) Upsert(ctx context.Context, id string, embedding []float64, meta map[string]string) error {
	f32 := float64To32(embedding)
	vec := pgvector.NewVector(f32)
	metaJSON, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	q := fmt.Sprintf(`INSERT INTO %s (id, embedding, meta) VALUES ($1, $2, $3)
		ON CONFLICT (id) DO UPDATE SET embedding = $2, meta = $3`, s.tableName)
	_, err = s.db.ExecContext(ctx, q, id, vec, string(metaJSON))
	return err
}

// Search returns the top-K most similar vectors using pgvector cosine distance.
func (s *PgVectorStore) Search(ctx context.Context, embedding []float64, topK int, minScore float64) ([]VectorHit, error) {
	if topK <= 0 {
		topK = 10
	}
	f32 := float64To32(embedding)
	vec := pgvector.NewVector(f32)
	q := fmt.Sprintf(`SELECT id, 1 - (embedding <=> $1) AS score, meta
		FROM %s
		WHERE 1 - (embedding <=> $1) >= $3
		ORDER BY embedding <=> $1
		LIMIT $2`, s.tableName)
	rows, err := s.db.QueryContext(ctx, q, vec, topK, minScore)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var hits []VectorHit
	for rows.Next() {
		var id string
		var score float64
		var metaStr string
		if err := rows.Scan(&id, &score, &metaStr); err != nil {
			return nil, err
		}
		var meta map[string]string
		_ = json.Unmarshal([]byte(metaStr), &meta)
		hits = append(hits, VectorHit{ID: id, Score: score, Meta: meta})
	}
	return hits, rows.Err()
}

// Delete removes a vector by ID.
func (s *PgVectorStore) Delete(ctx context.Context, id string) error {
	q := fmt.Sprintf(`DELETE FROM %s WHERE id = $1`, s.tableName)
	_, err := s.db.ExecContext(ctx, q, id)
	return err
}

func float64To32(v []float64) []float32 {
	out := make([]float32, len(v))
	for i, f := range v {
		out[i] = float32(f)
	}
	return out
}
