package data

import (
	"context"

	"aranea-agents/internal/data/ent"
	"aranea-agents/pkg/loggateway"
)

// ensureMemoryFactsIndexStatusPatches adds the MEM-OPT-01 Phase 0 columns to
// memory_facts on existing databases. New databases get them via memory_chain.sql.
//
// The 4 columns track external vector index (pgvector / embedding_blob)
// consistency so the read path and a future MemoryFactIndexReconciler cron can
// detect and re-sync stale facts after Cascade Approve / fact updates that
// fail to propagate to the index.
func ensureMemoryFactsIndexStatusPatches(ctx context.Context, c *ent.Client, lg loggateway.Logger) error {
	if c == nil {
		return nil
	}
	hasTable, err := sqliteTableExists(ctx, c, lg, "memory_facts")
	if err != nil {
		return err
	}
	if !hasTable {
		return nil
	}
	patches := []struct {
		column string
		ddl    string
	}{
		{"index_status", `ALTER TABLE memory_facts ADD COLUMN index_status TEXT NOT NULL DEFAULT 'fresh'`},
		{"index_synced_at", `ALTER TABLE memory_facts ADD COLUMN index_synced_at INTEGER NOT NULL DEFAULT 0`},
		{"index_attempts", `ALTER TABLE memory_facts ADD COLUMN index_attempts INTEGER NOT NULL DEFAULT 0`},
		{"index_last_error", `ALTER TABLE memory_facts ADD COLUMN index_last_error TEXT NOT NULL DEFAULT ''`},
	}
	for _, p := range patches {
		has, err := sqliteColumnExists(ctx, c, lg, "memory_facts", p.column)
		if err != nil {
			lg.Warn("memory facts index status patch check failed", loggateway.StepID("memory.schema_patch_fail"), loggateway.Err(err))
			return err
		}
		if has {
			continue
		}
		if _, err := c.ExecContext(ctx, p.ddl); err != nil {
			lg.Warn("memory facts index status patch ddl failed", loggateway.StepID("memory.schema_patch_fail"), loggateway.Str("column", p.column), loggateway.Err(err))
			return err
		}
	}
	if _, err := c.ExecContext(ctx,
		`CREATE INDEX IF NOT EXISTS idx_memory_facts_index_status ON memory_facts(index_status, index_synced_at)`); err != nil {
		lg.Warn("memory facts index status index create failed", loggateway.StepID("memory.schema_patch_fail"), loggateway.Err(err))
		return err
	}

	return nil
}

// ensureMemoryFactsExtraPatches adds pii_types and quality_score columns to
// memory_facts. Split from ensureMemoryFactsIndexStatusPatches so that if the
// original migration (20260602) was already recorded but these columns were
// missed (e.g. partial failure), this separate migration still runs.
func ensureMemoryFactsExtraPatches(ctx context.Context, c *ent.Client, lg loggateway.Logger) error {
	if c == nil {
		return nil
	}
	hasTable, err := sqliteTableExists(ctx, c, lg, "memory_facts")
	if err != nil {
		return err
	}
	if !hasTable {
		return nil
	}
	patches := []struct {
		column string
		ddl    string
	}{
		{"pii_types", `ALTER TABLE memory_facts ADD COLUMN pii_types TEXT NOT NULL DEFAULT ''`},
		{"quality_score", `ALTER TABLE memory_facts ADD COLUMN quality_score REAL NOT NULL DEFAULT 0`},
	}
	for _, p := range patches {
		has, err := sqliteColumnExists(ctx, c, lg, "memory_facts", p.column)
		if err != nil {
			lg.Warn("memory facts extra patch check failed", loggateway.StepID("memory.schema_patch_fail"), loggateway.Err(err))
			return err
		}
		if has {
			continue
		}
		if _, err := c.ExecContext(ctx, p.ddl); err != nil {
			lg.Warn("memory facts extra patch ddl failed", loggateway.StepID("memory.schema_patch_fail"), loggateway.Str("column", p.column), loggateway.Err(err))
			return err
		}
	}
	return nil
}
