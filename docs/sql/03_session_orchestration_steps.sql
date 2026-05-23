-- M53 Phase 5b: persisted orchestration activity timeline
CREATE TABLE IF NOT EXISTS orchestration_steps (
  id TEXT PRIMARY KEY,
  team_run_id TEXT NOT NULL DEFAULT '',
  graph_execution_id TEXT NOT NULL DEFAULT '',
  node_id TEXT NOT NULL DEFAULT '',
  activity_snapshot_json TEXT NOT NULL DEFAULT '{}',
  status TEXT NOT NULL DEFAULT '',
  started_at TEXT NOT NULL DEFAULT '',
  finished_at TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT ''
);
