package data

import (
	"context"
	"fmt"

	"aranea-agents/internal/data/ent"
	"aranea-agents/pkg/loggateway"
)

// RunCompressionDefaultOnMigration flips the compression-stack switches
// false→true for all existing agent_runtime_settings rows
// (N2, 2026-08-13 链路审查：历史无上限修复).
//
// Background: the 2026-08-13 LLM request-chain review found __spirit__
// averaging 60K prompt tokens (context-rot zone). Root cause: the compression
// cascade was built but its consumption side stayed disabled —
// session_summary_enabled=false meant the framework summary synced by the
// Compressor via EnqueueFrameworkSummary was never injected and history was
// never cut at the summary boundary; context_compaction_enabled=false meant
// the request-level compaction safety net never ran. Rows flipped here
// overwhelmingly carry the old schema default rather than a deliberate
// opt-out — the feature was effectively broken end-to-end, so no meaningful
// explicit disable exists to preserve. Users can still opt out per agent via
// the settings UI afterwards. memory_compact_enabled is included to align
// rows created before its schema default became true.
func RunCompressionDefaultOnMigration(ctx context.Context, client *ent.Client, d Dialect, lg loggateway.Logger) error {
	if client == nil {
		return fmt.Errorf("compression default-on migration: ent client required")
	}
	applied, err := isMigrationApplied(ctx, client, MigrationCompressionDefaultOn, lg)
	if err != nil {
		return fmt.Errorf("compression default-on migration: check gate: %w", err)
	}
	if applied {
		return nil
	}

	hasTable, err := tableExistsWithDialect(ctx, client, lg, "agent_runtime_settings", d)
	if err != nil {
		return fmt.Errorf("compression default-on migration: check table: %w", err)
	}
	if hasTable {
		res, err := client.ExecContext(ctx,
			`UPDATE agent_runtime_settings
			 SET context_compaction_enabled = true,
			     session_summary_enabled = true,
			     memory_compact_enabled = true
			 WHERE context_compaction_enabled = false
			    OR session_summary_enabled = false
			    OR memory_compact_enabled = false`)
		if err != nil {
			return fmt.Errorf("compression default-on migration: update: %w", err)
		}
		if n, _ := res.RowsAffected(); n > 0 {
			lg.Info("compression stack switches flipped to default-on",
				loggateway.StepID("migration.compression_default_on"),
				loggateway.Int("rows", int(n)))
		}
	}

	if err := recordMigrationApplied(ctx, client, d, MigrationCompressionDefaultOn, migrationNameCompressionDefaultOn, lg); err != nil {
		return fmt.Errorf("compression default-on migration: record: %w", err)
	}
	return nil
}
