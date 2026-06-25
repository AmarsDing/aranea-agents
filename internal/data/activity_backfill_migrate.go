package data

import (
	"context"
	"fmt"

	"aranea-agents/internal/data/ent"
	"aranea-agents/pkg/loggateway"
)

// RunActivityBackfillMigration historically reconstructed Activity records from
// ChatMessages for Pre-AF sessions. Phase 1c-3 deleted the messages table;
// backfill is now handled by SQL migration 20260902_drop_messages_subsystem.sql
// (INSERT INTO activities SELECT FROM messages WHERE NOT EXISTS ...).
//
// This function is retained only to record the migration gate for existing
// deployments that haven't run the original Go-based backfill. New deployments
// get backfill from the SQL migration.
func RunActivityBackfillMigration(ctx context.Context, client *ent.Client, d Dialect, lg loggateway.Logger) error {
	if client == nil {
		return fmt.Errorf("activity backfill migration: ent client required")
	}
	applied, err := isMigrationApplied(ctx, client, MigrationActivityBackfill, lg)
	if err != nil {
		return fmt.Errorf("activity backfill migration: check gate: %w", err)
	}
	if applied {
		return nil
	}
	// Record the gate so this no-op doesn't run again. The SQL migration
	// 20260902 handles the actual backfill idempotently.
	if err := recordMigrationApplied(ctx, client, d, MigrationActivityBackfill, migrationNameActivityBackfill, lg); err != nil {
		return fmt.Errorf("activity backfill migration: record: %w", err)
	}
	lg.Info("activity backfill (pre-AF): skipped (messages table removed; backfill handled by SQL migration 20260902)",
		loggateway.StepID("migration.activity_backfill"))
	return nil
}
