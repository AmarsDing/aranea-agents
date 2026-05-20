-- Plugin invocation audit rows (I2-PLG-01)

CREATE TABLE IF NOT EXISTS plugin_runs (
  id TEXT PRIMARY KEY,
  plugin_key TEXT NOT NULL,
  plugin_id TEXT NOT NULL DEFAULT '',
  session_id TEXT NOT NULL DEFAULT '',
  agent_id TEXT NOT NULL DEFAULT '',
  callback_point TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'ok',
  duration_ms INTEGER NOT NULL DEFAULT 0,
  detail_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_plugin_runs_plugin_key ON plugin_runs(plugin_key);
CREATE INDEX IF NOT EXISTS idx_plugin_runs_created_at ON plugin_runs(created_at);
