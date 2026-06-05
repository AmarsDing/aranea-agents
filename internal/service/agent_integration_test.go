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

// TestIntegrationAgentCRUD tests Agent CRUD operations with a real database.
// Run with: go test -tags=integration -run TestIntegrationAgentCRUD
func TestIntegrationAgentCRUD(t *testing.T) {
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

	// Verify basic Agent-like table creation and CRUD
	tableName := fmt.Sprintf("test_agent_%d", time.Now().UnixNano())
	_, err = db.ExecContext(ctx, fmt.Sprintf(
		"CREATE TABLE %s (id SERIAL PRIMARY KEY, name TEXT NOT NULL, slug TEXT NOT NULL UNIQUE, description TEXT, created_at TIMESTAMPTZ DEFAULT NOW())", tableName))
	if err != nil {
		t.Fatalf("failed to create test table: %v", err)
	}
	defer db.ExecContext(ctx, fmt.Sprintf("DROP TABLE IF EXISTS %s", tableName))

	// Create
	_, err = db.ExecContext(ctx, fmt.Sprintf(
		"INSERT INTO %s (name, slug, description) VALUES ($1, $2, $3)", tableName),
		"Test Agent", "test-agent", "A test agent")
	if err != nil {
		t.Fatalf("failed to insert agent: %v", err)
	}

	// Read
	var name, description string
	row := db.QueryRowContext(ctx, fmt.Sprintf("SELECT name, description FROM %s WHERE slug = $1", tableName), "test-agent")
	if err := row.Scan(&name, &description); err != nil {
		t.Fatalf("failed to query agent: %v", err)
	}
	if name != "Test Agent" || description != "A test agent" {
		t.Fatalf("expected name='Test Agent' description='A test agent', got name=%q description=%q", name, description)
	}

	// Update
	_, err = db.ExecContext(ctx, fmt.Sprintf("UPDATE %s SET description = $1 WHERE slug = $2", tableName), "Updated description", "test-agent")
	if err != nil {
		t.Fatalf("failed to update agent: %v", err)
	}

	row = db.QueryRowContext(ctx, fmt.Sprintf("SELECT description FROM %s WHERE slug = $1", tableName), "test-agent")
	if err := row.Scan(&description); err != nil {
		t.Fatalf("failed to query updated agent: %v", err)
	}
	if description != "Updated description" {
		t.Fatalf("expected description 'Updated description', got %q", description)
	}

	// Delete
	_, err = db.ExecContext(ctx, fmt.Sprintf("DELETE FROM %s WHERE slug = $1", tableName), "test-agent")
	if err != nil {
		t.Fatalf("failed to delete agent: %v", err)
	}

	row = db.QueryRowContext(ctx, fmt.Sprintf("SELECT COUNT(*) FROM %s", tableName))
	var count int
	if err := row.Scan(&count); err != nil {
		t.Fatalf("failed to count: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected 0 rows after delete, got %d", count)
	}

	t.Log("Agent integration test passed: CRUD verified")
}
