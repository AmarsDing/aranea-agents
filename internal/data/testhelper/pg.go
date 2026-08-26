package testhelper

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"net/url"
	"os"
	"strings"
	"testing"

	"aranea-agents/internal/data/ent"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/lib/pq"
)

// defaultTestPGDSN mirrors the local dev Postgres in configs/config.yaml but
// targets a dedicated aranea_test database so tests never touch dev data.
// Host 侧经 pg-proxy 55432 到 aranea-postgres（5432 被 twinserver-postgres 占用，
// 2026-08-27 端口漂移修正：密码同步为便携实例实际值）。
// Override with ARANEA_TEST_PG_DSN for CI or non-default environments.
const defaultTestPGDSN = "postgres://postgres:123456@127.0.0.1:55432/aranea_test?sslmode=disable"

// TestPGDSN resolves the Postgres DSN used by the test suite.
// ARANEA_TEST_PG_DSN wins; otherwise the local dev default is used.
func TestPGDSN() string {
	if dsn := strings.TrimSpace(os.Getenv("ARANEA_TEST_PG_DSN")); dsn != "" {
		return dsn
	}
	return defaultTestPGDSN
}

// SetupTestPG creates an isolated Postgres schema for one test, applies all
// Ent auto-migrations inside it, and returns the Ent client plus the raw
// *sql.DB bound to that schema via search_path. The schema is dropped
// (CASCADE) when the test finishes. Safe for parallel tests: every test gets
// its own schema inside the shared test database.
func SetupTestPG(t *testing.T) (*ent.Client, *sql.DB) {
	t.Helper()
	dsn := TestPGDSN()
	ctx := context.Background()

	ensureTestDatabase(t, dsn)

	schema := "test_" + randHex(t, 8)
	admin, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open test postgres: %v", err)
	}
	defer admin.Close()
	if _, err := admin.ExecContext(ctx, `CREATE SCHEMA `+pq.QuoteIdentifier(schema)); err != nil {
		t.Fatalf("create test schema %s: %v (is Postgres reachable at ARANEA_TEST_PG_DSN?)", schema, err)
	}

	db, err := sql.Open("postgres", withSearchPath(dsn, schema))
	if err != nil {
		t.Fatalf("open schema-scoped postgres: %v", err)
	}
	client := ent.NewClient(ent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	if err := client.Schema.Create(ctx); err != nil {
		db.Close()
		client.Close()
		t.Fatalf("ent schema create in %s: %v", schema, err)
	}

	t.Cleanup(func() {
		_ = client.Close()
		_ = db.Close()
		cleanupDB, err := sql.Open("postgres", dsn)
		if err != nil {
			return
		}
		defer cleanupDB.Close()
		_, _ = cleanupDB.ExecContext(context.Background(),
			`DROP SCHEMA IF EXISTS `+pq.QuoteIdentifier(schema)+` CASCADE`)
	})
	return client, db
}

// ensureTestDatabase creates the DSN's database when missing. Best-effort for
// URL-style DSNs; keyword-format DSNs are assumed to point at an existing
// database (a clear connect error surfaces otherwise).
func ensureTestDatabase(t *testing.T, dsn string) {
	t.Helper()
	u, err := url.Parse(dsn)
	if err != nil || u.Scheme == "" {
		return
	}
	dbName := strings.TrimPrefix(u.Path, "/")
	if dbName == "" || dbName == "postgres" {
		return
	}
	adminURL := *u
	adminURL.Path = "/postgres"
	admin, err := sql.Open("postgres", adminURL.String())
	if err != nil {
		t.Fatalf("open admin postgres: %v", err)
	}
	defer admin.Close()
	ctx := context.Background()
	var exists bool
	if err := admin.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname = $1)`, dbName).Scan(&exists); err != nil {
		t.Fatalf("check test database %s: %v", dbName, err)
	}
	if exists {
		return
	}
	if _, err := admin.ExecContext(ctx, `CREATE DATABASE `+pq.QuoteIdentifier(dbName)); err != nil {
		// 42P04 duplicate_database: a parallel test package created it first.
		if pqErr, ok := err.(*pq.Error); !ok || string(pqErr.Code) != "42P04" {
			t.Fatalf("create test database %s: %v", dbName, err)
		}
	}
}

// SetupTestPGRaw creates an isolated Postgres schema for one test WITHOUT
// applying Ent auto-migrations, returning the raw *sql.DB bound to that
// schema. Use this for raw-SQL repos whose production DDL (TEXT timestamps,
// BYTEA payloads, etc.) intentionally differs from the Ent schema — the test
// creates tables via its own DDL mirroring the DDL migration files.
func SetupTestPGRaw(t *testing.T) *sql.DB {
	t.Helper()
	dsn := TestPGDSN()
	ctx := context.Background()

	ensureTestDatabase(t, dsn)

	schema := "test_" + randHex(t, 8)
	admin, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open test postgres: %v", err)
	}
	defer admin.Close()
	if _, err := admin.ExecContext(ctx, `CREATE SCHEMA `+pq.QuoteIdentifier(schema)); err != nil {
		t.Fatalf("create test schema %s: %v (is Postgres reachable at ARANEA_TEST_PG_DSN?)", schema, err)
	}

	db, err := sql.Open("postgres", withSearchPath(dsn, schema))
	if err != nil {
		t.Fatalf("open schema-scoped postgres: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
		cleanupDB, err := sql.Open("postgres", dsn)
		if err != nil {
			return
		}
		defer cleanupDB.Close()
		_, _ = cleanupDB.ExecContext(context.Background(),
			`DROP SCHEMA IF EXISTS `+pq.QuoteIdentifier(schema)+` CASCADE`)
	})
	return db
}

// withSearchPath returns the DSN with search_path pinned to the given schema.
// lib/pq forwards unknown parameters to the backend as runtime settings.
func withSearchPath(dsn, schema string) string {
	if u, err := url.Parse(dsn); err == nil && u.Scheme != "" {
		q := u.Query()
		q.Set("search_path", schema)
		u.RawQuery = q.Encode()
		return u.String()
	}
	return dsn + " search_path=" + schema
}

func randHex(t *testing.T, n int) string {
	t.Helper()
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		t.Fatalf("rand: %v", err)
	}
	return hex.EncodeToString(b)
}
