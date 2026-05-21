-- embedded by flow_log_schema.go
CREATE TABLE IF NOT EXISTS flow_log_events (
  id TEXT PRIMARY KEY,
  trace_id TEXT NOT NULL DEFAULT '',
  session_id TEXT NOT NULL DEFAULT '',
  run_id TEXT NOT NULL DEFAULT '',
  team_id TEXT NOT NULL DEFAULT '',
  domain TEXT NOT NULL DEFAULT '',
  agent_key TEXT NOT NULL DEFAULT '',
  step_id TEXT NOT NULL DEFAULT '',
  flow_phase TEXT NOT NULL DEFAULT '',
  severity TEXT NOT NULL DEFAULT 'info',
  title TEXT NOT NULL DEFAULT '',
  message TEXT NOT NULL DEFAULT '',
  payload_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_flow_log_trace_created ON flow_log_events(trace_id, created_at);
CREATE INDEX IF NOT EXISTS idx_flow_log_session_created ON flow_log_events(session_id, created_at);
CREATE INDEX IF NOT EXISTS idx_flow_log_run_created ON flow_log_events(run_id, created_at);
