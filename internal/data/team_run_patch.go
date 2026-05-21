package data

import (
	"context"

	"aranea-agents/internal/data/ent"
)

func ensureTeamRunSummaryPatches(ctx context.Context, c *ent.Client) error {
	if c == nil {
		return nil
	}
	has, err := sqliteColumnExists(ctx, c, "team_runs", "summary_json")
	if err != nil {
		return err
	}
	if has {
		return nil
	}
	_, err = c.ExecContext(ctx, `ALTER TABLE team_runs ADD COLUMN summary_json TEXT NOT NULL DEFAULT ''`)
	return err
}
