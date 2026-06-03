//go:build integration

package service

import (
	"context"
	"testing"

	"aranea-agents/internal/testutil"
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

	// TODO: Wire up a real server with the test database and test Chat API endpoints.
	// This requires creating a full server instance with the test DSN, which depends
	// on the project's wire injection setup. The test container is ready for use.
	t.Log("Integration test container started successfully")
}
