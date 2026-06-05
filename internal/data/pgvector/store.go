package pgvector

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/pgvector/pgvector-go"
)

var (
	ErrDBUnavailable = errors.New("pgvector store: postgres not available")
	ErrDimMismatch   = errors.New("pgvector store: embedding length mismatch")
)

// Row is one persisted agent_memory_* row.
type Row struct {
	ID        int64
	AgentID   string
	UserID    string
	Content   string
	Distance  float64
	CreatedAt time.Time
}

// Store uses a fixed-dimension table agent_memory_<dim>.
//
// Deprecated: Use internal/data/vector/pgvector.go instead. This package is
// retained for backward compatibility and will not receive new features.
type Store struct {
	db    *sql.DB
	dim   int
	table string
}

// NewStore binds to a dimension-specific table (EnsureDimensionTable must have been called for dim).
func NewStore(db *sql.DB, dim int) *Store {
	if dim <= 0 {
		dim = 1536
	}
	return &Store{db: db, dim: dim, table: TableNameForDimension(dim)}
}

func (s *Store) Dim() int {
	if s == nil || s.dim <= 0 {
		return 1536
	}
	return s.dim
}

func (s *Store) Table() string {
	if s == nil || s.table == "" {
		return TableNameForDimension(s.Dim())
	}
	return s.table
}

func (s *Store) dbOrErr() (*sql.DB, error) {
	if s == nil || s.db == nil {
		return nil, ErrDBUnavailable
	}
	return s.db, nil
}

func (s *Store) expectDim(v []float32) error {
	if len(v) != s.Dim() {
		return fmt.Errorf("%w: got %d want %d", ErrDimMismatch, len(v), s.Dim())
	}
	return nil
}

func (s *Store) Insert(ctx context.Context, agentID, userID, content string, embedding []float32) error {
	db, err := s.dbOrErr()
	if err != nil {
		return err
	}
	if err := s.expectDim(embedding); err != nil {
		return err
	}
	vec := pgvector.NewVector(embedding)
	q := fmt.Sprintf(`INSERT INTO %s (agent_id, user_id, content, embedding) VALUES ($1, $2, $3, $4)`, s.Table())
	_, err = db.ExecContext(ctx, q, agentID, userID, content, vec)
	return err
}

func (s *Store) Get(ctx context.Context, id int64) (*Row, error) {
	db, err := s.dbOrErr()
	if err != nil {
		return nil, err
	}
	q := fmt.Sprintf(`SELECT id, agent_id, user_id, content, created_at FROM %s WHERE id = $1`, s.Table())
	var r Row
	err = db.QueryRowContext(ctx, q, id).
		Scan(&r.ID, &r.AgentID, &r.UserID, &r.Content, &r.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func (s *Store) Update(ctx context.Context, id int64, agentID, content string, embedding []float32) error {
	db, err := s.dbOrErr()
	if err != nil {
		return err
	}
	if err := s.expectDim(embedding); err != nil {
		return err
	}
	vec := pgvector.NewVector(embedding)
	q := fmt.Sprintf(`UPDATE %s SET content = $1, embedding = $2 WHERE id = $3 AND agent_id = $4`, s.Table())
	res, err := db.ExecContext(ctx, q, content, vec, id, agentID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) Delete(ctx context.Context, id int64) error {
	db, err := s.dbOrErr()
	if err != nil {
		return err
	}
	q := fmt.Sprintf(`DELETE FROM %s WHERE id = $1`, s.Table())
	res, err := db.ExecContext(ctx, q, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) SearchNearest(ctx context.Context, agentID, userID string, queryEmbedding []float32, limit int) ([]Row, error) {
	db, err := s.dbOrErr()
	if err != nil {
		return nil, err
	}
	if err := s.expectDim(queryEmbedding); err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 10
	}
	vec := pgvector.NewVector(queryEmbedding)
	q := fmt.Sprintf(`
SELECT id, agent_id, user_id, content, embedding <=> $3::vector AS distance, created_at
FROM %s
WHERE agent_id = $1 AND user_id = $2
ORDER BY embedding <=> $3::vector
LIMIT $4`, s.Table())
	rows, err := db.QueryContext(ctx, q, agentID, userID, vec, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Row
	for rows.Next() {
		var r Row
		if err := rows.Scan(&r.ID, &r.AgentID, &r.UserID, &r.Content, &r.Distance, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// FactVectorContentPrefix returns the stable pgvector content prefix for one fact id.
func FactVectorContentPrefix(factID string) string {
	return "fact_id:" + strings.TrimSpace(factID) + "\n"
}

// ParseFactVectorContent splits pgvector content into fact id and statement text.
func ParseFactVectorContent(content string) (factID, statement string) {
	content = strings.TrimSpace(content)
	if !strings.HasPrefix(content, "fact_id:") {
		return "", content
	}
	rest := content[len("fact_id:"):]
	if i := strings.IndexByte(rest, '\n'); i >= 0 {
		return strings.TrimSpace(rest[:i]), strings.TrimSpace(rest[i+1:])
	}
	return strings.TrimSpace(rest), ""
}

func factVectorContent(factID, statement string) string {
	return FactVectorContentPrefix(factID) + strings.TrimSpace(statement)
}

// UpsertFactVector replaces the pgvector read-index row for a memory_facts id.
func (s *Store) UpsertFactVector(ctx context.Context, agentID, userID, factID, statement string, embedding []float32) error {
	db, err := s.dbOrErr()
	if err != nil {
		return err
	}
	if err := s.expectDim(embedding); err != nil {
		return err
	}
	prefix := FactVectorContentPrefix(factID)
	delQ := fmt.Sprintf(`DELETE FROM %s WHERE agent_id = $1 AND user_id = $2 AND content LIKE $3`, s.Table())
	if _, err := db.ExecContext(ctx, delQ, agentID, userID, prefix+"%"); err != nil {
		return err
	}
	vec := pgvector.NewVector(embedding)
	insQ := fmt.Sprintf(`INSERT INTO %s (agent_id, user_id, content, embedding) VALUES ($1, $2, $3, $4)`, s.Table())
	_, err = db.ExecContext(ctx, insQ, agentID, userID, factVectorContent(factID, statement), vec)
	return err
}
