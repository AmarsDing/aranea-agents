package data

import (
	"context"
	"database/sql"
)

// EnsureChatTurnSubmitClaimSchema creates chat_turn_submit_claim for durable
// chat submit/enqueue idempotency (survives process restart). Mirrors
// channel_inbound_receipt: UNIQUE(session_id, request_id) + ON CONFLICT DO NOTHING.
func EnsureChatTurnSubmitClaimSchema(ctx context.Context, db *sql.DB) error {
	if db == nil {
		return nil
	}
	_, err := db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS chat_turn_submit_claim (
  id TEXT PRIMARY KEY,
  session_id TEXT NOT NULL,
  request_id TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT '',
  UNIQUE(session_id, request_id)
);
CREATE INDEX IF NOT EXISTS idx_chat_turn_submit_claim_created
  ON chat_turn_submit_claim(created_at);
`)
	return err
}
