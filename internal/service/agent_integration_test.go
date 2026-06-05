//go:build integration

package service

import (
	"context"
	"testing"

	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/agent"
	"aranea-agents/internal/data/ent/migrate"
	"aranea-agents/internal/testutil"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "github.com/lib/pq"
)

// TestIntegrationAgentCRUD tests Agent CRUD operations with a real PostgreSQL container.
// Run with: go test -tags=integration ./internal/service/... -run TestIntegrationAgentCRUD -count=1
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

	// Test: Create an agent
	ag, err := client.Agent.Create().
		SetID("test-agent-int-001").
		SetAgentKey("integration-test-agent").
		SetDisplayName("Integration Test Agent").
		SetKind(agent.KindUser).
		SetProvider("openai").
		SetModel("gpt-4").
		Save(ctx)
	if err != nil {
		t.Fatalf("failed to create agent: %v", err)
	}
	t.Logf("Created agent: id=%s display_name=%s kind=%s", ag.ID, ag.DisplayName, ag.Kind)

	// Assert: Agent was created with correct fields
	if ag.DisplayName != "Integration Test Agent" {
		t.Fatalf("expected agent display_name 'Integration Test Agent', got %q", ag.DisplayName)
	}
	if ag.Kind != "user" {
		t.Fatalf("expected agent kind 'user', got %q", ag.Kind)
	}

	// Test: Retrieve agent by ID
	retrieved, err := client.Agent.Get(ctx, ag.ID)
	if err != nil {
		t.Fatalf("failed to retrieve agent: %v", err)
	}
	if retrieved.DisplayName != ag.DisplayName {
		t.Fatalf("expected retrieved display_name %q, got %q", ag.DisplayName, retrieved.DisplayName)
	}

	// Test: Query agents by agent_key
	agents, err := client.Agent.Query().
		Where(agent.AgentKey("integration-test-agent")).
		All(ctx)
	if err != nil {
		t.Fatalf("failed to query agents by agent_key: %v", err)
	}
	if len(agents) != 1 {
		t.Fatalf("expected 1 agent with agent_key 'integration-test-agent', got %d", len(agents))
	}

	// Test: Update agent display_name
	updated, err := client.Agent.UpdateOneID(ag.ID).
		SetDisplayName("Updated Integration Agent").
		Save(ctx)
	if err != nil {
		t.Fatalf("failed to update agent: %v", err)
	}
	if updated.DisplayName != "Updated Integration Agent" {
		t.Fatalf("expected updated display_name 'Updated Integration Agent', got %q", updated.DisplayName)
	}

	// Test: Delete agent
	err = client.Agent.DeleteOneID(ag.ID).Exec(ctx)
	if err != nil {
		t.Fatalf("failed to delete agent: %v", err)
	}

	// Assert: Agent no longer exists
	_, err = client.Agent.Get(ctx, ag.ID)
	if err == nil {
		t.Fatal("expected error when getting deleted agent, got nil")
	}
	t.Log("Agent integration test passed: Agent CRUD verified via Ent Client")
}
