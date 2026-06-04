-- Self-check report persistence for monitor-selfcheck-repair module.
CREATE TABLE IF NOT EXISTS self_check_reports (
  id TEXT PRIMARY KEY,
  check_results_json TEXT NOT NULL DEFAULT '[]',
  overall_status TEXT NOT NULL DEFAULT 'passed',
  repair_actions_json TEXT NOT NULL DEFAULT '[]',
  started_at TEXT NOT NULL,
  finished_at TEXT NOT NULL,
  duration_ms INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_self_check_reports_status_created ON self_check_reports(overall_status, created_at);
CREATE INDEX IF NOT EXISTS idx_self_check_reports_created ON self_check_reports(created_at);
