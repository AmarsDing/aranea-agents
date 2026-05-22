package data

import (
	"context"
	"database/sql"
)

// EnsureChannelInboundSchema creates channel_inbound_receipt for inbound idempotency.
func EnsureChannelInboundSchema(ctx context.Context, db *sql.DB) error {
	if db == nil {
		return nil
	}
	_, err := db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS channel_inbound_receipt (
  id TEXT PRIMARY KEY,
  channel_id TEXT NOT NULL,
  idempotency_key TEXT NOT NULL,
  peer_id TEXT NOT NULL DEFAULT '',
  text_preview TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT '',
  UNIQUE(channel_id, idempotency_key)
);
CREATE INDEX IF NOT EXISTS idx_channel_inbound_receipt_channel_created
  ON channel_inbound_receipt(channel_id, created_at);
`)
	return err
}
