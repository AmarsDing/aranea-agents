-- embedded by skill_evolution_schema.go
CREATE TABLE IF NOT EXISTS skill_proposals (
  id TEXT PRIMARY KEY,
  agent_id TEXT NOT NULL,
  pattern_hash TEXT NOT NULL DEFAULT '',
  pattern_desc TEXT NOT NULL DEFAULT '',
  skill_name TEXT NOT NULL DEFAULT '',
  skill_md TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'pending',
  approved_by TEXT NOT NULL DEFAULT '',
  rejected_by TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  approved_at TEXT
);

CREATE INDEX IF NOT EXISTS idx_sprop_agent_status ON skill_proposals(agent_id, status);
CREATE INDEX IF NOT EXISTS idx_sprop_pattern_hash ON skill_proposals(agent_id, pattern_hash);
