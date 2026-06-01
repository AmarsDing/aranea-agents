-- embedded by plan_schema.go
CREATE TABLE IF NOT EXISTS plans (
  id TEXT PRIMARY KEY,
  session_id TEXT NOT NULL DEFAULT '',
  agent_key TEXT NOT NULL DEFAULT '',
  goal TEXT NOT NULL DEFAULT '',
  steps_json TEXT NOT NULL DEFAULT '[]',
  status TEXT NOT NULL DEFAULT 'draft',
  surface_id TEXT NOT NULL DEFAULT '',
  graph_id TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_plans_session_created ON plans(session_id, created_at);
CREATE INDEX IF NOT EXISTS idx_plans_status ON plans(status);
