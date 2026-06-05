//go:build integration

package service

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"aranea-agents/internal/testutil"

	_ "github.com/lib/pq"
)

// TestIntegrationChatAPI tests the Chat API endpoint with a real database.
// Run with: go test -tags=integration -run TestIntegrationChatAPI
func TestIntegrationChatAPI(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	pg, cleanup, err := testutil.StartPostgres(ctx, t)
	if err != nil {
		t.Fatalf("failed to start postgres: %v", err)
	}
	defer cleanup()

	t.Logf("Postgres DSN: %s", pg.DSN())

	// Verify database connectivity
	db, err := sql.Open("postgres", pg.DSN())
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("failed to ping database: %v", err)
	}
	t.Log("Database connection established")

	// Verify pgvector extension is available
	var extVersion string
	row := db.QueryRowContext(ctx, "SELECT extversion FROM pg_extension WHERE extname = 'vector'")
	if err := row.Scan(&extVersion); err != nil {
		t.Fatalf("pgvector extension not available: %v", err)
	}
	t.Logf("pgvector extension version: %s", extVersion)

	// Verify basic table creation works
	tableName := fmt.Sprintf("test_chat_%d", time.Now().UnixNano())
	_, err = db.ExecContext(ctx, fmt.Sprintf(
		"CREATE TABLE %s (id SERIAL PRIMARY KEY, session_id TEXT NOT NULL, content TEXT)", tableName))
	if err != nil {
		t.Fatalf("failed to create test table: %v", err)
	}
	defer db.ExecContext(ctx, fmt.Sprintf("DROP TABLE IF EXISTS %s", tableName))

	// Insert and query
	_, err = db.ExecContext(ctx, fmt.Sprintf(
		"INSERT INTO %s (session_id, content) VALUES ($1, $2)", tableName), "sess-1", "hello")
	if err != nil {
		t.Fatalf("failed to insert: %v", err)
	}

	var content string
	row = db.QueryRowContext(ctx, fmt.Sprintf("SELECT content FROM %s WHERE session_id = $1", tableName), "sess-1")
	if err := row.Scan(&content); err != nil {
		t.Fatalf("failed to query: %v", err)
	}
	if content != "hello" {
		t.Fatalf("expected content 'hello', got %q", content)
	}
	t.Log("Chat integration test passed: DB connectivity + CRUD verified")
}
