package data_test

import (
	"context"
	"testing"

	"aranea-agents/internal/data"
	"aranea-agents/internal/data/sessionmemory"
	"aranea-agents/pkg/loggateway"
)

// openTestStoreWithData creates a sessionmemory.Store and a *Data sharing the same
// SQLite connection, so that schema_migrations and memory_* tables are visible to both.
// It reuses openTestSessionMemoryStore from trpc_memory_facts_test.go for the Store,
// then wraps the same client in a Data for migration helpers.
func openTestStoreWithData(t *testing.T) (*sessionmemory.Store, *data.Data) {
	t.Helper()
	store, client := openTestSessionMemoryStoreWithClient(t)
	d := &data.Data{}
	d.SetEntClientForTest(client)
	return store, d
}

func TestRunLegacyTRPCMemoryMigration_versionGate(t *testing.T) {
	store, d := openTestStoreWithData(t)
	ctx := context.Background()

	client := d.ClientFromCtx(ctx)
	_, err := client.ExecContext(ctx, `
INSERT INTO memory_entities (
 id, scope_type, scope_id, workspace_id, user_id,
 entity_type, name, name_normalized, aliases_json, description, attributes_json,
 importance, confidence, use_count, source_kind,
 embedding_status, embedding_model, embedding_dim, embedding_blob, embedding_norm,
 status, merged_into, metadata_json, created_at, updated_at, archived_at, deleted_at
) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		"leg-gate", "trpc_memory", "agent-gate", "", "user-gate",
		"memory_fact", "Gate test fact", "gate", "[]", "Gate test fact", "{}",
		0.7, 0.85, 0, "legacy",
		"pending", "", 0, nil, 0.0,
		"active", "", "{}", "2026-01-01T00:00:00Z", "2026-01-01T00:00:00Z", "", "",
	)
	if err != nil {
		t.Fatal(err)
	}

	migrated, skipped, err := data.RunLegacyTRPCMemoryMigration(ctx, store, d, loggateway.NewNoop())
	if err != nil {
		t.Fatal(err)
	}
	if skipped {
		t.Fatal("expected first run not skipped")
	}
	if migrated != 1 {
		t.Fatalf("expected migrated=1, got %d", migrated)
	}

	migrated, skipped, err = data.RunLegacyTRPCMemoryMigration(ctx, store, d, loggateway.NewNoop())
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
	store, d := openTestStoreWithData(t)
	ctx := context.Background()

	client := d.ClientFromCtx(ctx)
	_, err := client.ExecContext(ctx, `
INSERT INTO memory_entities (
 id, scope_type, scope_id, workspace_id, user_id,
 entity_type, name, name_normalized, aliases_json, description, attributes_json,
 importance, confidence, use_count, source_kind,
 embedding_status, embedding_model, embedding_dim, embedding_blob, embedding_norm,
 status, merged_into, metadata_json, created_at, updated_at, archived_at, deleted_at
) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		"leg-invalid", "trpc_memory", "", "", "user-x",
		"memory_fact", "", "invalid", "[]", "", "{}",
		0.5, 0.85, 0, "legacy",
		"pending", "", 0, nil, 0.0,
		"active", "", "{}", "2026-01-01T00:00:00Z", "2026-01-01T00:00:00Z", "", "",
	)
	if err != nil {
		t.Fatal(err)
	}

	migrated, skipped, err := data.RunLegacyTRPCMemoryMigration(ctx, store, d, loggateway.NewNoop())
	if err != nil {
		t.Fatal(err)
	}
	if skipped {
		t.Fatal("expected migration to run, not skip")
	}
	if migrated != 0 {
		t.Fatalf("expected migrated=0, got %d", migrated)
	}

	migrated, skipped, err = data.RunLegacyTRPCMemoryMigration(ctx, store, d, loggateway.NewNoop())
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
