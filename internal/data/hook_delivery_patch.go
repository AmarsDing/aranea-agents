package data

import (
	"context"

	"aranea-agents/internal/data/ent"
)

// ensureHookDeliveryPatches applies incremental column additions to hook_deliveries
// for existing installs where the table was created before these columns existed.
func ensureHookDeliveryPatches(ctx context.Context, c *ent.Client) error {
	if c == nil {
		return nil
	}
	patches := []struct {
		col string
		ddl string
	}{
		{"webhook_secret", `ALTER TABLE hook_deliveries ADD COLUMN webhook_secret TEXT NOT NULL DEFAULT ''`},
		{"idempotency_key", `ALTER TABLE hook_deliveries ADD COLUMN idempotency_key TEXT NOT NULL DEFAULT ''`},
	}
	for _, p := range patches {
		has, err := sqliteColumnExists(ctx, c, "hook_deliveries", p.col)
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
	indexPatches := []struct {
		name string
		ddl  string
	}{
		{
			"idx_hook_deliveries_idempotency",
			`CREATE UNIQUE INDEX IF NOT EXISTS idx_hook_deliveries_idempotency ON hook_deliveries(idempotency_key) WHERE idempotency_key <> ''`,
		},
	}
	for _, p := range indexPatches {
		has, err := sqliteIndexExists(ctx, c, "hook_deliveries", p.name)
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
	return nil
}
