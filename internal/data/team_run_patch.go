package data

import (
	"context"

	"aranea-agents/internal/data/ent"
)

func ensureTeamRunSummaryPatches(ctx context.Context, c *ent.Client) error {
	if c == nil {
		return nil
	}
	if err := ensureTeamRunColumn(ctx, c, "summary_json", `ALTER TABLE team_runs ADD COLUMN summary_json TEXT NOT NULL DEFAULT ''`); err != nil {
		return err
	}
	return ensureTeamRunColumn(ctx, c, "definition_snapshot_json", `ALTER TABLE team_runs ADD COLUMN definition_snapshot_json TEXT NOT NULL DEFAULT ''`)
}

func ensureTeamRunColumn(ctx context.Context, c *ent.Client, column, ddl string) error {
	has, err := sqliteColumnExists(ctx, c, "team_runs", column)
	if err != nil {
		return err
	}
	if has {
		return nil
	}
	_, err = c.ExecContext(ctx, ddl)
	return err
}
