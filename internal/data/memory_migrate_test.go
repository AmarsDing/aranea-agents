package data_test

import (
	"context"
	"testing"

	"aranea-agents/internal/data"
	"aranea-agents/pkg/loggateway"
)

func TestRunLegacyTRPCMemoryMigration_versionGate(t *testing.T) {
	store := openTestSessionMemoryStore(t)
	ctx := context.Background()

	_, err := store.Client().ExecContext(ctx, `
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

	migrated, skipped, err := data.RunLegacyTRPCMemoryMigration(ctx, store, loggateway.NewNoop())
	if err != nil {
		t.Fatal(err)
	}
	if skipped {
		t.Fatal("expected first run not skipped")
	}
	if migrated != 1 {
		t.Fatalf("expected migrated=1, got %d", migrated)
	}

	migrated, skipped, err = data.RunLegacyTRPCMemoryMigration(ctx, store, loggateway.NewNoop())
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
	store := openTestSessionMemoryStore(t)
	ctx := context.Background()

	_, err := store.Client().ExecContext(ctx, `
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

	migrated, skipped, err := data.RunLegacyTRPCMemoryMigration(ctx, store, loggateway.NewNoop())
	if err != nil {
		t.Fatal(err)
	}
	if skipped {
		t.Fatal("expected migration to run, not skip")
	}
	if migrated != 0 {
		t.Fatalf("expected migrated=0, got %d", migrated)
	}

	migrated, skipped, err = data.RunLegacyTRPCMemoryMigration(ctx, store, loggateway.NewNoop())
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
