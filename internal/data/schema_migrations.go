package data

import (
	"context"
	"fmt"
	"time"

	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/schemamigration"
)

const (
	MigrationLegacyTRPCMemoryFacts     = 20260524
	migrationNameLegacyTRPCMemoryFacts = "legacy_trpc_memory_facts"
	MigrationTurnIndexToTurnID         = 20260528
	migrationNameTurnIndexToTurnID     = "turn_index_to_turn_id"
	MigrationSessionStatusIdle         = 20260531
	migrationNameSessionStatusIdle     = "session_status_active_to_idle"
)

func isMigrationApplied(ctx context.Context, client *ent.Client, version int) (bool, error) {
	if client == nil {
		return false, fmt.Errorf("schema migrations: ent client required")
	}
	return client.SchemaMigration.Query().Where(schemamigration.ID(version)).Exist(ctx)
}

func recordMigrationApplied(ctx context.Context, client *ent.Client, version int, name string) error {
	if client == nil {
		return fmt.Errorf("schema migrations: ent client required")
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := client.SchemaMigration.Create().
		SetID(version).
		SetName(name).
		SetAppliedAt(now).
		Save(ctx)
	return err
}

func IsSeedApplied(ctx context.Context, client *ent.Client, version int) (bool, error) {
	return isMigrationApplied(ctx, client, version)
}

func MarkSeedApplied(ctx context.Context, client *ent.Client, version int, name string) error {
	return recordMigrationApplied(ctx, client, version, name)
}
