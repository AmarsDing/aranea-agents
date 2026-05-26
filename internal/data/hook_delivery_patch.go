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
	return nil
}
