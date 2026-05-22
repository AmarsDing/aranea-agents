package data

import (
	"context"
	"database/sql"
)

// EnsureChannelTurnJobSchema creates channel_turn_job for inbound turn auditing.
func EnsureChannelTurnJobSchema(ctx context.Context, db *sql.DB) error {
	if db == nil {
		return nil
	}
	_, err := db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS channel_turn_job (
  id TEXT PRIMARY KEY,
  channel_id TEXT NOT NULL,
  session_id TEXT NOT NULL DEFAULT '',
  peer_id TEXT NOT NULL DEFAULT '',
  peer_key TEXT NOT NULL DEFAULT '',
  idempotency_key TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'accepted',
  preview_message_id TEXT NOT NULL DEFAULT '',
  content_preview TEXT NOT NULL DEFAULT '',
  async_target_type TEXT NOT NULL DEFAULT '',
  async_target_id TEXT NOT NULL DEFAULT '',
  error_message TEXT NOT NULL DEFAULT '',
  started_at TEXT NOT NULL DEFAULT '',
  finished_at TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL DEFAULT '',
  UNIQUE(channel_id, idempotency_key)
);
CREATE INDEX IF NOT EXISTS idx_channel_turn_job_channel_created
  ON channel_turn_job(channel_id, created_at);
CREATE INDEX IF NOT EXISTS idx_channel_turn_job_session
  ON channel_turn_job(session_id);
`)
	return err
}
