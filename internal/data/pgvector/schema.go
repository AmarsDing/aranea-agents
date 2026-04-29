package pgvector

import (
	"context"
	"database/sql"
	"fmt"
)

// EnsureExtension creates the pgvector extension if missing (once per DB).
func EnsureExtension(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, `CREATE EXTENSION IF NOT EXISTS vector`); err != nil {
		return fmt.Errorf("create extension vector (ensure DB role may create extensions): %w", err)
	}
	return nil
}

// TableNameForDimension returns the partitioned table name for embeddings of fixed width dim.
// pgvector column type vector(dim) is fixed; different models ⇒ different dims ⇒ different tables.
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

// EnsureSchema applies extension plus the default singleton table agent_memory_<defaultDim>.
// Prefer EnsureExtension + EnsureDimensionTable per distinct dim for multi-model setups.
func EnsureSchema(ctx context.Context, db *sql.DB, dim int) error {
	if err := EnsureExtension(ctx, db); err != nil {
		return err
	}
	return EnsureDimensionTable(ctx, db, dim)
}
