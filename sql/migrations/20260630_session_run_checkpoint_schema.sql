-- Version 20260630: Session run checkpoint schema
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
