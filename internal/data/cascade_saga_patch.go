package data

import (
	"context"

	"aranea-agents/internal/data/ent"
	"aranea-agents/pkg/loggateway"
)

func ensureCascadeSagaPatches(ctx context.Context, c *ent.Client, lg loggateway.Logger) error {
	if c == nil {
		return nil
	}
	if err := ensureCascadeSagaStepsTable(ctx, c, lg); err != nil {
		return err
	}
	return ensureCascadeOriginalStatementColumn(ctx, c, lg)
}

func ensureCascadeSagaStepsTable(ctx context.Context, c *ent.Client, lg loggateway.Logger) error {
	has, err := sqliteTableExists(ctx, c, lg, "cascade_saga_steps")
	if err != nil {
		return err
	}
	if has {
		return nil
	}
	_, err = c.ExecContext(ctx, `
CREATE TABLE cascade_saga_steps (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    proposal_id TEXT NOT NULL,
    step_index INTEGER NOT NULL,
    step_name TEXT NOT NULL,
    state TEXT NOT NULL DEFAULT 'pending'
        CHECK(state IN ('pending','running','succeeded','failed','compensated','skipped')),
    is_critical INTEGER NOT NULL DEFAULT 1,
    attempts INTEGER NOT NULL DEFAULT 0,
    started_at TEXT,
    finished_at TEXT,
    payload_json TEXT,
    result_json TEXT,
    error TEXT,
    UNIQUE(proposal_id, step_index)
)`)
	if err != nil {
		return err
	}
	_, err = c.ExecContext(ctx,
		`CREATE INDEX IF NOT EXISTS idx_cascade_saga_steps_proposal ON cascade_saga_steps(proposal_id, step_index)`)
	return err
}

func ensureCascadeOriginalStatementColumn(ctx context.Context, c *ent.Client, lg loggateway.Logger) error {
	hasTable, err := sqliteTableExists(ctx, c, lg, "memory_facts")
	if err != nil {
		return err
	}
	if !hasTable {
		return nil
	}
	has, err := sqliteColumnExists(ctx, c, lg, "memory_facts", "last_cascade_original_statement")
	if err != nil {
		return err
	}
	if has {
		return nil
	}
	_, err = c.ExecContext(ctx,
		`ALTER TABLE memory_facts ADD COLUMN last_cascade_original_statement TEXT NOT NULL DEFAULT ''`)
	return err
}
