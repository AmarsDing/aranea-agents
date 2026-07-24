package data_test

import (
	"context"
	"testing"

	"aranea-agents/internal/data"
	"aranea-agents/pkg/loggateway"
)

// openTestDataForMigration creates a *Data backed by a schema-isolated
// Postgres test DB for migration testing. It reuses openTestDataForMemory
// from trpc_memory_facts_test.go.
func openTestDataForMigration(t *testing.T) *data.Data {
	t.Helper()
	d, _ := openTestDataForMemory(t)
	return d
}

func TestRunLegacyTRPCMemoryMigration_versionGate(t *testing.T) {
	d := openTestDataForMigration(t)
	ctx := context.Background()

	client := d.ClientFromCtx(ctx)
	// Create the trpc_memory_entities table that the migration code queries.
	if _, err := client.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS trpc_memory_entities (
 id TEXT PRIMARY KEY, scope_type TEXT NOT NULL, scope_id TEXT NOT NULL DEFAULT '', user_id TEXT NOT NULL DEFAULT '',
 agent_id TEXT NOT NULL DEFAULT '', entity_type TEXT NOT NULL, name TEXT NOT NULL,
 statement TEXT NOT NULL DEFAULT '', details TEXT NOT NULL DEFAULT '',
 confidence REAL NOT NULL DEFAULT 0.7, importance REAL NOT NULL DEFAULT 0.5,
 source_kind TEXT NOT NULL DEFAULT '', source_session_id TEXT NOT NULL DEFAULT '',
 source_message_id TEXT NOT NULL DEFAULT '', metadata_json TEXT NOT NULL DEFAULT '{}',
 created_at TEXT NOT NULL, migrated INTEGER NOT NULL DEFAULT 0
)`); err != nil {
		t.Fatal(err)
	}
	_, err := client.ExecContext(ctx, pgRebind(`
INSERT INTO trpc_memory_entities (
 id, scope_type, scope_id, user_id, agent_id, entity_type, name, statement, details,
 confidence, importance, source_kind, source_session_id, source_message_id, metadata_json, created_at, migrated
) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`),
		"leg-gate", "trpc_memory", "agent-gate", "user-gate", "agent-gate",
		"memory_fact", "Gate test fact", "Gate test fact statement", "Gate test fact",
		0.7, 0.85, "legacy", "", "", "{}", "2026-01-01T00:00:00Z", 0,
	)
	if err != nil {
		t.Fatal(err)
	}

	migrated, skipped, err := data.RunLegacyTRPCMemoryMigration(ctx, d, loggateway.NewNoop())
	if err != nil {
		t.Fatal(err)
	}
	if skipped {
		t.Fatal("expected first run not skipped")
	}
	if migrated != 1 {
		t.Fatalf("expected migrated=1, got %d", migrated)
	}

	migrated, skipped, err = data.RunLegacyTRPCMemoryMigration(ctx, d, loggateway.NewNoop())
	if err != nil {
		t.Fatal(err)
	}
	if !skipped {
		t.Fatal("expected second run skipped by migration gate")
	}
	if migrated != 0 {
		t.Fatalf("expected migrated=0 when skipped, got %d", migrated)
	}
}

func TestRunLegacyTRPCMemoryMigration_completesWithSkippedInvalid(t *testing.T) {
	d := openTestDataForMigration(t)
	ctx := context.Background()

	client := d.ClientFromCtx(ctx)
	// Create the trpc_memory_entities table that the migration code queries.
	if _, err := client.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS trpc_memory_entities (
 id TEXT PRIMARY KEY, scope_type TEXT NOT NULL, scope_id TEXT NOT NULL DEFAULT '', user_id TEXT NOT NULL DEFAULT '',
 agent_id TEXT NOT NULL DEFAULT '', entity_type TEXT NOT NULL, name TEXT NOT NULL,
 statement TEXT NOT NULL DEFAULT '', details TEXT NOT NULL DEFAULT '',
 confidence REAL NOT NULL DEFAULT 0.7, importance REAL NOT NULL DEFAULT 0.5,
 source_kind TEXT NOT NULL DEFAULT '', source_session_id TEXT NOT NULL DEFAULT '',
 source_message_id TEXT NOT NULL DEFAULT '', metadata_json TEXT NOT NULL DEFAULT '{}',
 created_at TEXT NOT NULL, migrated INTEGER NOT NULL DEFAULT 0
)`); err != nil {
		t.Fatal(err)
	}
	_, err := client.ExecContext(ctx, pgRebind(`
INSERT INTO trpc_memory_entities (
 id, scope_type, scope_id, user_id, agent_id, entity_type, name, statement, details,
 confidence, importance, source_kind, source_session_id, source_message_id, metadata_json, created_at, migrated
) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`),
		"leg-invalid", "trpc_memory", "", "user-x", "",
		"memory_fact", "", "", "",
		0.5, 0.85, "legacy", "", "", "{}", "2026-01-01T00:00:00Z", 0,
	)
	if err != nil {
		t.Fatal(err)
	}

	migrated, skipped, err := data.RunLegacyTRPCMemoryMigration(ctx, d, loggateway.NewNoop())
	if err != nil {
		t.Fatal(err)
	}
	if skipped {
		t.Fatal("expected migration to run, not skip")
	}
	if migrated != 0 {
		t.Fatalf("expected migrated=0, got %d", migrated)
	}

	migrated, skipped, err = data.RunLegacyTRPCMemoryMigration(ctx, d, loggateway.NewNoop())
	if err != nil {
		t.Fatal(err)
	}
	if !skipped {
		t.Fatal("expected second run skipped after gate recorded")
	}
	if migrated != 0 {
		t.Fatalf("expected migrated=0 when skipped, got %d", migrated)
	}
}
