package data

import (
	"context"
	"database/sql"

	"aranea-agents/internal/event"
)

func ensureSessionRunCheckpointSchema(ctx context.Context, db *sql.DB) error {
	if db == nil {
		return nil
	}
	_, err := db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS session_run_checkpoints (
  id TEXT PRIMARY KEY,
  session_run_id TEXT NOT NULL,
  session_id TEXT NOT NULL,
  turn_id TEXT NOT NULL DEFAULT '',
  agent_id TEXT NOT NULL DEFAULT '',
  payload_json TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_session_run_checkpoints_run
  ON session_run_checkpoints(session_run_id);
`)
	return err
}

func ensureSessionRunColumnPatches(ctx context.Context, db *sql.DB) error {
	if db == nil {
		return nil
	}
	patches := []string{
		`ALTER TABLE session_runs ADD COLUMN checkpoint_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE session_runs ADD COLUMN agent_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE session_runs ADD COLUMN resume_started_at TEXT NOT NULL DEFAULT ''`,
	}
	for _, q := range patches {
		if _, execErr := db.ExecContext(ctx, q); execErr != nil {
			event.SysLogDebug("session_run.schema_patch", "ddl patch skipped", event.P("query", q), event.P("error", execErr.Error()))
		}
	}
	return nil
}
