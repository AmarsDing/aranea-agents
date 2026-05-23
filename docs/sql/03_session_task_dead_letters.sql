-- M53 Phase 6 FP-04: dead-letter queue for halted team/graph jobs
CREATE TABLE IF NOT EXISTS task_dead_letters (
  id TEXT PRIMARY KEY,
  source_type TEXT NOT NULL DEFAULT '',
  source_id TEXT NOT NULL DEFAULT '',
  team_id TEXT NOT NULL DEFAULT '',
  team_run_id TEXT NOT NULL DEFAULT '',
  session_id TEXT NOT NULL DEFAULT '',
  graph_execution_id TEXT NOT NULL DEFAULT '',
  error_message TEXT NOT NULL DEFAULT '',
  payload_json TEXT NOT NULL DEFAULT '{}',
  status TEXT NOT NULL DEFAULT 'pending',
  created_at TEXT NOT NULL DEFAULT '',
  resolved_at TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_task_dead_letters_status_created
  ON task_dead_letters (status, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_task_dead_letters_team_run
  ON task_dead_letters (team_run_id);
