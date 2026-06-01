package data

import (
	"context"
	"fmt"

	"aranea-agents/internal/data/ent"
	"aranea-agents/pkg/loggateway"
)

func RunSessionStatusIdleMigration(ctx context.Context, client *ent.Client) error {
	if client == nil {
		return fmt.Errorf("session status migration: ent client required")
	}
	applied, err := isMigrationApplied(ctx, client, MigrationSessionStatusIdle)
	if err != nil {
		return fmt.Errorf("session status migration: check gate: %w", err)
	}
	if applied {
		return nil
	}
	loggateway.Global().Info("session status: active→idle + NULL defaults: starting", loggateway.StepID("migration.session_status"))

	hasTable, err := sqliteTableExists(ctx, client, "sessions")
	if err != nil {
		return fmt.Errorf("session status migration: check table: %w", err)
	}
	if !hasTable {
		if err := recordMigrationApplied(ctx, client, MigrationSessionStatusIdle, migrationNameSessionStatusIdle); err != nil {
			return fmt.Errorf("session status migration: record: %w", err)
		}
		return nil
	}

	if _, err := client.ExecContext(ctx,
		`UPDATE sessions SET status = 'idle' WHERE status = 'active'`); err != nil {
		return fmt.Errorf("session status migration: active→idle: %w", err)
	}

	if _, err := client.ExecContext(ctx,
		`UPDATE sessions SET status_reason = '' WHERE status_reason IS NULL`); err != nil {
		return fmt.Errorf("session status migration: status_reason NULL→empty: %w", err)
	}

	if _, err := client.ExecContext(ctx,
		`UPDATE sessions SET status_changed_at = '' WHERE status_changed_at IS NULL`); err != nil {
		return fmt.Errorf("session status migration: status_changed_at NULL→empty: %w", err)
	}

	if err := recordMigrationApplied(ctx, client, MigrationSessionStatusIdle, migrationNameSessionStatusIdle); err != nil {
		return fmt.Errorf("session status migration: record: %w", err)
	}
	loggateway.Global().Info("session status: active→idle + NULL defaults: done", loggateway.StepID("migration.session_status"))
	return nil
}
