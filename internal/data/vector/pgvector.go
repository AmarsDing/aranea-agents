//go:build pgvector

package vector

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
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

var _ FilteredVectorStore = (*PgVectorStore)(nil)

// pgVectorMetaFilterKeys are the meta keys SearchFiltered accepts. Each one
// has a btree expression index (ensureTable) so a filtered search reads only
// the matching rows instead of scanning every vector in the table.
var pgVectorMetaFilterKeys = []string{"user_id", "agent_id"}

var pgVectorMetaKeyRe = regexp.MustCompile(`^[a-z_][a-z0-9_]*$`)

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
	if _, err := s.db.ExecContext(ctx, ddl); err != nil {
		return err
	}
	// Filter indexes are an optimisation: SearchFiltered stays correct (only
	// slower) without them, so a failure here must not block startup.
	for _, key := range pgVectorMetaFilterKeys {
		idx := fmt.Sprintf(`CREATE INDEX IF NOT EXISTS %s ON %s ((meta->>'%s'))`,
			pgVectorMetaIndexName(s.tableName, key), s.tableName, key)
		if _, err := s.db.ExecContext(ctx, idx); err != nil && s.lg != nil {
			s.lg.Warn("create vector meta filter index failed; filtered search falls back to a table scan",
				loggateway.StepID("data.vector"), loggateway.Str("meta_key", key), loggateway.Err(err))
		}
	}
	return nil
}

func pgVectorMetaIndexName(tableName, key string) string {
	return strings.ReplaceAll(tableName, ".", "_") + "_meta_" + key + "_idx"
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
	return s.SearchFiltered(ctx, embedding, topK, minScore, nil)
}

// SearchFiltered returns the top-K most similar vectors among rows whose
// meta[key] equals value for every filter entry (nil/empty filter = whole
// table). Keys are validated and inlined so the planner can match the
// (meta->>'key') expression indexes; values stay bound parameters.
func (s *PgVectorStore) SearchFiltered(ctx context.Context, embedding []float64, topK int, minScore float64, filter map[string]string) ([]VectorHit, error) {
	if topK <= 0 {
		topK = 10
	}
	f32 := float64To32(embedding)
	vec := pgvector.NewVector(f32)
	args := []any{vec, topK, minScore}
	where := []string{"1 - (embedding <=> $1) >= $3"}
	keys := make([]string, 0, len(filter))
	for k := range filter {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if !pgVectorMetaKeyRe.MatchString(k) {
			return nil, fmt.Errorf("vector search: invalid meta filter key %q", k)
		}
		args = append(args, filter[k])
		where = append(where, fmt.Sprintf("meta->>'%s' = $%d", k, len(args)))
	}
	q := fmt.Sprintf(`SELECT id, 1 - (embedding <=> $1) AS score, meta
		FROM %s
		WHERE %s
		ORDER BY embedding <=> $1
		LIMIT $2`, s.tableName, strings.Join(where, " AND "))
	rows, err := s.db.QueryContext(ctx, q, args...)
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
