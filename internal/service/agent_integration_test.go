//go:build integration

package service

import (
	"context"
	"testing"

	"aranea-agents/internal/testutil"
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

	// TODO: Wire up a real server with the test database and test Agent CRUD.
	t.Log("Integration test container started successfully")
}
