-- Hook notify delivery queue (see docs/sql/28_callback_delivery.sql)

CREATE TABLE IF NOT EXISTS hook_deliveries (
  id TEXT PRIMARY KEY,
  hook_key TEXT NOT NULL,
  hook_id TEXT NOT NULL DEFAULT '',
  webhook_url TEXT NOT NULL,
  payload_json TEXT NOT NULL DEFAULT '{}',
  status TEXT NOT NULL DEFAULT 'pending',
  attempt_count INTEGER NOT NULL DEFAULT 0,
  max_attempts INTEGER NOT NULL DEFAULT 3,
  last_error TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_hook_deliveries_hook_key ON hook_deliveries(hook_key);
CREATE INDEX IF NOT EXISTS idx_hook_deliveries_status ON hook_deliveries(status);
CREATE INDEX IF NOT EXISTS idx_hook_deliveries_created_at ON hook_deliveries(created_at);
