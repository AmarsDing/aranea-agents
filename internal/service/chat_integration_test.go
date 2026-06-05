//go:build integration

package service

import (
	"context"
	"testing"

	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/migrate"
	"aranea-agents/internal/data/ent/session"
	"aranea-agents/internal/testutil"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "github.com/lib/pq"
)

// TestIntegrationChatAPI tests Chat-related database operations with a real PostgreSQL container.
// Run with: go test -tags=integration ./internal/service/... -run TestIntegrationChatAPI -count=1
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

	// Create Ent Client with PostgreSQL driver
	drv, err := entsql.Open(dialect.Postgres, pg.DSN())
	if err != nil {
		t.Fatalf("failed to open ent driver: %v", err)
	}
	client := ent.NewClient(ent.Driver(drv))
	defer client.Close()

	// Run schema migration
	if err := client.Schema.Create(ctx, migrate.WithDropIndex(true)); err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}
	t.Log("Ent schema migration completed")

	// Test: Create a session
	sess, err := client.Session.Create().
		SetID("test-session-int-001").
		SetTitle("Integration Test Session").
		SetAgentID("test-agent").
		Save(ctx)
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}
	t.Logf("Created session: id=%s title=%s", sess.ID, sess.Title)

	// Assert: Session was created with correct fields
	if sess.Title != "Integration Test Session" {
		t.Fatalf("expected session title 'Integration Test Session', got %q", sess.Title)
	}
	if sess.ID != "test-session-int-001" {
		t.Fatalf("expected session id 'test-session-int-001', got %q", sess.ID)
	}

	// Test: Retrieve session by ID
	retrieved, err := client.Session.Get(ctx, sess.ID)
	if err != nil {
		t.Fatalf("failed to retrieve session: %v", err)
	}
	if retrieved.Title != sess.Title {
		t.Fatalf("expected retrieved title %q, got %q", sess.Title, retrieved.Title)
	}

	// Test: Query sessions by title
	sessions, err := client.Session.Query().
		Where(session.Title("Integration Test Session")).
		All(ctx)
	if err != nil {
		t.Fatalf("failed to query sessions by title: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session with title 'Integration Test Session', got %d", len(sessions))
	}

	// Test: Update session title
	updated, err := client.Session.UpdateOneID(sess.ID).
		SetTitle("Updated Integration Session").
		Save(ctx)
	if err != nil {
		t.Fatalf("failed to update session: %v", err)
	}
	if updated.Title != "Updated Integration Session" {
		t.Fatalf("expected updated title 'Updated Integration Session', got %q", updated.Title)
	}

	// Test: Delete session
	err = client.Session.DeleteOneID(sess.ID).Exec(ctx)
	if err != nil {
		t.Fatalf("failed to delete session: %v", err)
	}

	// Assert: Session no longer exists
	_, err = client.Session.Get(ctx, sess.ID)
	if err == nil {
		t.Fatal("expected error when getting deleted session, got nil")
	}
	t.Log("Chat integration test passed: Session CRUD verified via Ent Client")
}
