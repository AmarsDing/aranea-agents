package data

import (
	"context"

	"aranea-agents/internal/data/ent"
)

func ensureA2ARemoteHealthPatches(ctx context.Context, c *ent.Client) error {
	if c == nil {
		return nil
	}
	patches := []struct {
		col string
		ddl string
	}{
		{"last_health_at", `ALTER TABLE a2a_remote_agents ADD COLUMN last_health_at TEXT NOT NULL DEFAULT ''`},
		{"last_health_ok", `ALTER TABLE a2a_remote_agents ADD COLUMN last_health_ok INTEGER NOT NULL DEFAULT 0`},
		{"last_health_error", `ALTER TABLE a2a_remote_agents ADD COLUMN last_health_error TEXT NOT NULL DEFAULT ''`},
	}
	for _, p := range patches {
		has, err := sqliteColumnExists(ctx, c, "a2a_remote_agents", p.col)
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
