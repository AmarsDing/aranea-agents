package data

import (
	"context"

	"aranea-agents/internal/data/ent"
	"aranea-agents/pkg/loggateway"
)

func ensureTeamRunSummaryPatches(ctx context.Context, c *ent.Client, lg loggateway.Logger) error {
	if c == nil {
		return nil
	}
	if err := ensureTeamRunColumn(ctx, c, lg, "summary_json", `ALTER TABLE team_runs ADD COLUMN summary_json TEXT NOT NULL DEFAULT ''`); err != nil {
		return err
	}
	return ensureTeamRunColumn(ctx, c, lg, "definition_snapshot_json", `ALTER TABLE team_runs ADD COLUMN definition_snapshot_json TEXT NOT NULL DEFAULT ''`)
}

func ensureTeamRunColumn(ctx context.Context, c *ent.Client, lg loggateway.Logger, column, ddl string) error {
	has, err := sqliteColumnExists(ctx, c, lg, "team_runs", column)
	if err != nil {
		return err
	}
	if has {
		return nil
	}
	_, err = c.ExecContext(ctx, ddl)
	return err
}
