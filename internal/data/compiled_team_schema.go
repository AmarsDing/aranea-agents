package data

import (
	"context"
	"database/sql"
)

func EnsureCompiledTeamSchema(ctx context.Context, db *sql.DB) error {
	if db == nil {
		return nil
	}
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS compiled_teams (
  id TEXT PRIMARY KEY,
  team_id TEXT NOT NULL DEFAULT '',
  graph_id TEXT NOT NULL DEFAULT '',
  session_id TEXT NOT NULL DEFAULT '',
  config_json TEXT NOT NULL DEFAULT '',
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME
)`,
		`CREATE INDEX IF NOT EXISTS idx_compiled_teams_team_id ON compiled_teams(team_id)`,
		`CREATE INDEX IF NOT EXISTS idx_compiled_teams_graph_id ON compiled_teams(graph_id)`,
		`CREATE INDEX IF NOT EXISTS idx_compiled_teams_session_id ON compiled_teams(session_id)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_compiled_teams_team_graph ON compiled_teams(team_id, graph_id)`,
	}
	for _, s := range stmts {
		if _, err := db.ExecContext(ctx, s); err != nil {
			return err
		}
	}
	return nil
}
