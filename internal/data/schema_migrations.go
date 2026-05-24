package data

import (
	"context"
	"fmt"
	"time"

	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/schemamigration"
)

const (
	// MigrationLegacyTRPCMemoryFacts marks one-time trpc_memory entity → memory_facts backfill.
	MigrationLegacyTRPCMemoryFacts     = 20260524
	migrationNameLegacyTRPCMemoryFacts = "legacy_trpc_memory_facts"
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
