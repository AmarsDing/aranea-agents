package data

import (
	"context"
	"fmt"

	"aranea-agents/internal/data/sessionmemory"
)

// LegacyTRPCMigrationStatus reports pending legacy rows and whether the version gate passed.
type LegacyTRPCMigrationStatus struct {
	Pending int
	Applied bool
}

// GetLegacyTRPCMigrationStatus returns counts of unmigrated trpc_memory entities and gate state.
func GetLegacyTRPCMigrationStatus(ctx context.Context, store *sessionmemory.Store) (LegacyTRPCMigrationStatus, error) {
	var out LegacyTRPCMigrationStatus
	if store == nil {
		return out, fmt.Errorf("legacy trpc migration: store required")
	}
	client := store.Client()
	if client == nil {
		return out, fmt.Errorf("legacy trpc migration: ent client required")
	}
	applied, err := isMigrationApplied(ctx, client, MigrationLegacyTRPCMemoryFacts)
	if err != nil {
		return out, err
	}
	out.Applied = applied
	if applied {
		return out, nil
	}
	pending, err := store.CountPendingLegacyTRPCMemoryEntities(ctx)
	if err != nil {
		return out, err
	}
	out.Pending = pending
	return out, nil
}

// RunLegacyTRPCMemoryMigration backfills legacy trpc_memory entities once (schema_migrations gate).
func RunLegacyTRPCMemoryMigration(ctx context.Context, store *sessionmemory.Store) (migrated int, skipped bool, err error) {
	if store == nil {
		return 0, false, fmt.Errorf("legacy trpc migration: store required")
	}
	client := store.Client()
	if client == nil {
		return 0, false, fmt.Errorf("legacy trpc migration: ent client required")
	}
	applied, err := isMigrationApplied(ctx, client, MigrationLegacyTRPCMemoryFacts)
	if err != nil {
		return 0, false, err
	}
	if applied {
		return 0, true, nil
	}
	migrated, _, err = store.BackfillLegacyTRPCMemoryEntities(ctx)
	if err != nil {
		return migrated, false, err
	}
	remaining, err := store.CountPendingLegacyTRPCMemoryEntities(ctx)
	if err != nil {
		return migrated, false, err
	}
	if remaining > 0 {
		return migrated, false, fmt.Errorf("legacy trpc migration: %d entities still pending after backfill", remaining)
	}
	if err := recordMigrationApplied(ctx, client, MigrationLegacyTRPCMemoryFacts, migrationNameLegacyTRPCMemoryFacts); err != nil {
		return migrated, false, err
	}
	return migrated, false, nil
}
