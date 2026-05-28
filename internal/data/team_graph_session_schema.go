package data

import (
	"context"
	"database/sql"
)

func EnsureTeamGraphSessionSchema(ctx context.Context, db *sql.DB) error {
	if db == nil {
		return nil
	}
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS team_graph_sessions (
  exec_id TEXT PRIMARY KEY,
  team_run_id TEXT NOT NULL DEFAULT '',
  team_id TEXT NOT NULL DEFAULT '',
  session_id TEXT NOT NULL DEFAULT '',
  input_preview TEXT NOT NULL DEFAULT '',
  definition_json TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'running',
  registered_at TEXT NOT NULL DEFAULT '',
  last_activity_at TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL DEFAULT ''
)`,
		`CREATE INDEX IF NOT EXISTS idx_team_graph_sessions_team_run ON team_graph_sessions(team_run_id)`,
		`CREATE INDEX IF NOT EXISTS idx_team_graph_sessions_status ON team_graph_sessions(status)`,
	}
	for _, s := range stmts {
		if _, err := db.ExecContext(ctx, s); err != nil {
			return err
		}
	}
	return nil
}
