package data

import (
	"context"

	"aranea-agents/internal/data/ent"
)

// ensureMemoryFactsIndexStatusPatches adds the MEM-OPT-01 Phase 0 columns to
// memory_facts on existing databases. New databases get them via memory_chain.sql.
//
// The 4 columns track external vector index (pgvector / embedding_blob)
// consistency so the read path and a future MemoryFactIndexReconciler cron can
// detect and re-sync stale facts after Cascade Approve / fact updates that
// fail to propagate to the index.
func ensureMemoryFactsIndexStatusPatches(ctx context.Context, c *ent.Client) error {
	if c == nil {
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
		has, err := sqliteColumnExists(ctx, c, "memory_facts", p.column)
		if err != nil {
			return err
		}
		if has {
			continue
		}
		if _, err := c.ExecContext(ctx, p.ddl); err != nil {
			return err
		}
	}
	if _, err := c.ExecContext(ctx,
		`CREATE INDEX IF NOT EXISTS idx_memory_facts_index_status ON memory_facts(index_status, index_synced_at)`); err != nil {
		return err
	}
	return nil
}
