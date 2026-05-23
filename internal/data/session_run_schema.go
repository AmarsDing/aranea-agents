package data

import (
	"context"
	"database/sql"
)

// EnsureSessionRunSchema creates session_runs for M55 Run lifecycle (CC-R-01).
func EnsureSessionRunSchema(ctx context.Context, db *sql.DB) error {
	if db == nil {
		return nil
	}
	_, err := db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS session_runs (
  id TEXT PRIMARY KEY,
  session_id TEXT NOT NULL,
  turn_id TEXT NOT NULL DEFAULT '',
  runtime_run_id TEXT NOT NULL DEFAULT '',
  source TEXT NOT NULL DEFAULT '',
  phase TEXT NOT NULL DEFAULT 'interactive',
  soft_budget_sec INTEGER NOT NULL DEFAULT 180,
  hard_budget_sec INTEGER NOT NULL DEFAULT 900,
  checkpoint_id TEXT NOT NULL DEFAULT '',
  workflow_job_id TEXT NOT NULL DEFAULT '',
  agent_id TEXT NOT NULL DEFAULT '',
  error_message TEXT NOT NULL DEFAULT '',
  started_at TEXT NOT NULL DEFAULT '',
  phase_changed_at TEXT NOT NULL DEFAULT '',
  finished_at TEXT NOT NULL DEFAULT '',
  resume_started_at TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_session_runs_session_created
  ON session_runs(session_id, created_at);
CREATE INDEX IF NOT EXISTS idx_session_runs_turn
  ON session_runs(session_id, turn_id);
`)
	return err
}
