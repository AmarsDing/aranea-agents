package data

import (
	"context"
	"fmt"

	"aranea-agents/internal/data/ent"
	"aranea-agents/pkg/loggateway"
)

// RunL2RecallDefaultOnMigration flips l2_recall_enabled false→true for all
// existing agent_runtime_settings rows (FR-12/P2: L2 召回默认开).
//
// Background: the 2026-07-29 memory redesign review (V7) identified
// "default-off L2 recall" as one of the stacked defaults that made the memory
// layers write-only in practice. The standard memory tier (profile card +
// L2/L3 recall) requires L2 recall on. Rows flipped here overwhelmingly carry
// the old schema default rather than a deliberate opt-out — the feature was
// broken end-to-end, so no meaningful explicit disable exists to preserve.
// Users can still opt out per agent via the settings UI afterwards.
func RunL2RecallDefaultOnMigration(ctx context.Context, client *ent.Client, d Dialect, lg loggateway.Logger) error {
	if client == nil {
		return fmt.Errorf("l2 recall default-on migration: ent client required")
	}
	applied, err := isMigrationApplied(ctx, client, MigrationL2RecallDefaultOn, lg)
	if err != nil {
		return fmt.Errorf("l2 recall default-on migration: check gate: %w", err)
	}
	if applied {
		return nil
	}

	hasTable, err := tableExistsWithDialect(ctx, client, lg, "agent_runtime_settings", d)
	if err != nil {
		return fmt.Errorf("l2 recall default-on migration: check table: %w", err)
	}
	if hasTable {
		res, err := client.ExecContext(ctx,
			`UPDATE agent_runtime_settings SET l2_recall_enabled = true WHERE l2_recall_enabled = false`)
		if err != nil {
			return fmt.Errorf("l2 recall default-on migration: update: %w", err)
		}
		if n, _ := res.RowsAffected(); n > 0 {
			lg.Info("l2_recall_enabled flipped to default-on",
				loggateway.StepID("migration.l2_recall_default_on"),
				loggateway.Int("rows", int(n)))
		}
	}

	if err := recordMigrationApplied(ctx, client, d, MigrationL2RecallDefaultOn, migrationNameL2RecallDefaultOn, lg); err != nil {
		return fmt.Errorf("l2 recall default-on migration: record: %w", err)
	}
	return nil
}
