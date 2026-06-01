package data

import (
	"context"

	"aranea-agents/internal/data/ent"
	"aranea-agents/pkg/loggateway"
)

func ensureEntityReinforcementsSchema(ctx context.Context, c *ent.Client, lg loggateway.Logger) error {
	if c == nil {
		return nil
	}
	_, err := c.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS entity_reinforcements (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  entity_id TEXT NOT NULL,
  signal TEXT NOT NULL CHECK(signal IN ('hit','confirmed','refuted','edited')),
  occurred_at INTEGER NOT NULL,
  source TEXT NOT NULL DEFAULT ''
)`)
	if err != nil {
		lg.Warn("entity reinforcements create table failed", loggateway.StepID("memory.schema_init_fail"), loggateway.Err(err))
		return err
	}
	c.ExecContext(ctx, `
CREATE INDEX IF NOT EXISTS idx_entity_reinforcements_entity_time
  ON entity_reinforcements(entity_id, occurred_at DESC)`)
	return nil
}
