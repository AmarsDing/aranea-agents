package app

import (
	"context"
	"database/sql"
)

// MigrationSource describes one Context's migration directory. The runner
// applies them in name order under a per-Context module identifier so that
// schema_migrations.module distinguishes Contexts.
//
// Skeleton state (P0): type definition only; the actual driver lives in
// kernel/pkg/db (to be migrated by row #30).
type MigrationSource struct {
	Module string // Context identifier, e.g. "identity"
	Path   string // filesystem or embed path to *.up.sql
}

// RunMigrations applies every registered MigrationSource against db. Stub
// state (P0): no-op. Real implementation lands with kernel/pkg/db migration.
func RunMigrations(ctx context.Context, db *sql.DB, sources []MigrationSource) error {
	_ = ctx
	_ = db
	_ = sources
	return nil
}
