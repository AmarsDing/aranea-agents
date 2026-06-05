package testhelper

import (
	"context"
	"database/sql"
	"testing"

	"aranea-agents/internal/data/ent"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"

	_ "github.com/glebarez/go-sqlite/compat"
)

// SetupTestDB creates an in-memory SQLite database with all Ent auto-migrations applied.
// Returns the Ent client and the underlying raw *sql.DB for DDL migration testing.
func SetupTestDB(t *testing.T) (*ent.Client, *sql.DB) {
	t.Helper()
	rawDB, err := sql.Open(dialect.SQLite, "file:enttest?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatalf("failed to open test sqlite: %v", err)
	}
	t.Cleanup(func() { rawDB.Close() })

	for _, pragma := range []string{
		"PRAGMA foreign_keys=ON",
		"PRAGMA journal_mode=WAL",
	} {
		if _, e := rawDB.ExecContext(context.Background(), pragma); e != nil {
			t.Fatalf("pragma %s: %v", pragma, e)
		}
	}

	drv := entsql.OpenDB(dialect.SQLite, rawDB)
	client := ent.NewClient(ent.Driver(drv))
	t.Cleanup(func() { client.Close() })

	if err := client.Schema.Create(context.Background()); err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}

	return client, rawDB
}
