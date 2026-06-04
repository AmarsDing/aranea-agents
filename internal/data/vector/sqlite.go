package vector

import (
	"context"
	"database/sql"
	"encoding/json"
	"math"
	"sort"
	"strings"

	"aranea-agents/pkg/loggateway"
)

// SQLiteVectorStore implements VectorStore using SQLite JSON columns and Go-side cosine similarity.
type SQLiteVectorStore struct {
	db        *sql.DB
	tableName string
	lg        loggateway.Logger
}

// NewSQLiteVectorStore creates a new SQLiteVectorStore and ensures the table exists.
func NewSQLiteVectorStore(db *sql.DB, tableName string, lg loggateway.Logger) (*SQLiteVectorStore, error) {
	if db == nil {
		return nil, sql.ErrConnDone
	}
	tableName = strings.TrimSpace(tableName)
	if tableName == "" {
		tableName = "vector_embeddings"
	}
	s := &SQLiteVectorStore{db: db, tableName: tableName, lg: lg}
	if err := s.ensureTable(context.Background()); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *SQLiteVectorStore) ensureTable(ctx context.Context) error {
	ddl := `CREATE TABLE IF NOT EXISTS ` + s.tableName + ` (
		id TEXT PRIMARY KEY,
		embedding_json TEXT NOT NULL,
		meta_json TEXT NOT NULL DEFAULT '{}'
	)`
	_, err := s.db.ExecContext(ctx, ddl)
	return err
}

// Upsert inserts or replaces a vector embedding for the given ID.
func (s *SQLiteVectorStore) Upsert(ctx context.Context, id string, embedding []float64, meta map[string]string) error {
	embJSON, err := json.Marshal(embedding)
	if err != nil {
		return err
	}
	metaJSON, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT OR REPLACE INTO `+s.tableName+` (id, embedding_json, meta_json) VALUES (?, ?, ?)`,
		id, string(embJSON), string(metaJSON))
	return err
}

// Search reads all vectors, computes cosine similarity on the Go side, and returns top-K hits.
func (s *SQLiteVectorStore) Search(ctx context.Context, embedding []float64, topK int, minScore float64) ([]VectorHit, error) {
	if topK <= 0 {
		topK = 10
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, embedding_json, meta_json FROM `+s.tableName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var candidates []VectorHit
	for rows.Next() {
		var id, embStr, metaStr string
		if err := rows.Scan(&id, &embStr, &metaStr); err != nil {
			return nil, err
		}
		var stored []float64
		if err := json.Unmarshal([]byte(embStr), &stored); err != nil {
			continue
		}
		score := cosineSim64(embedding, stored)
		if score < minScore {
			continue
		}
		var meta map[string]string
		_ = json.Unmarshal([]byte(metaStr), &meta)
		candidates = append(candidates, VectorHit{ID: id, Score: score, Meta: meta})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	sort.Slice(candidates, func(i, j int) bool { return candidates[i].Score > candidates[j].Score })
	if len(candidates) > topK {
		candidates = candidates[:topK]
	}
	return candidates, nil
}

// Delete removes a vector by ID.
func (s *SQLiteVectorStore) Delete(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM `+s.tableName+` WHERE id = ?`, id)
	return err
}

func cosineSim64(a, b []float64) float64 {
	if len(a) == 0 || len(a) != len(b) {
		return 0
	}
	var dot, na, nb float64
	for i := range a {
		dot += a[i] * b[i]
		na += a[i] * a[i]
		nb += b[i] * b[i]
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}
