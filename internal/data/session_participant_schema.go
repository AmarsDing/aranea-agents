package data

import (
	"context"
	"database/sql"

	"aranea-agents/pkg/loggateway"
)

func EnsureSessionParticipantSchema(ctx context.Context, db *sql.DB, lg loggateway.Logger) error {
	if db == nil {
		return nil
	}
	_, err := db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS session_participants (
  id TEXT PRIMARY KEY,
  session_id TEXT NOT NULL,
  participant_type TEXT NOT NULL DEFAULT 'agent',
  participant_id TEXT NOT NULL DEFAULT '',
  display_name TEXT NOT NULL DEFAULT '',
  role_in_session TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'active',
  first_active_at TEXT NOT NULL DEFAULT '',
  last_active_at TEXT NOT NULL DEFAULT '',
  message_count INTEGER NOT NULL DEFAULT 0,
  run_step_count INTEGER NOT NULL DEFAULT 0,
  input_tokens INTEGER NOT NULL DEFAULT 0,
  output_tokens INTEGER NOT NULL DEFAULT 0,
  context_used_ratio REAL NOT NULL DEFAULT 0,
  metadata_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL DEFAULT '',
  UNIQUE(session_id, participant_type, participant_id, role_in_session)
);
CREATE INDEX IF NOT EXISTS idx_session_participants_session
  ON session_participants(session_id);
`)
	if err != nil {
		lg.Error("create session_participants table failed", loggateway.StepID("data.session_participant.schema.create"), loggateway.Err(err))
	}
	return err
}
