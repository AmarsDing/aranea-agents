-- embedded by learning_loop_schema.go
CREATE TABLE IF NOT EXISTS learning_observations (
  id TEXT PRIMARY KEY,
  agent_id TEXT NOT NULL,
  session_id TEXT NOT NULL DEFAULT '',
  kind TEXT NOT NULL DEFAULT 'tool_call',
  content TEXT NOT NULL DEFAULT '',
  metadata TEXT NOT NULL DEFAULT '',
  observed_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_lobs_agent_observed ON learning_observations(agent_id, observed_at);
CREATE INDEX IF NOT EXISTS idx_lobs_kind ON learning_observations(kind);

CREATE TABLE IF NOT EXISTS learning_patterns (
  id TEXT PRIMARY KEY,
  agent_id TEXT NOT NULL,
  kind TEXT NOT NULL DEFAULT '',
  description TEXT NOT NULL DEFAULT '',
  frequency INTEGER NOT NULL DEFAULT 0,
  confidence REAL NOT NULL DEFAULT 0,
  evidence TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'detected',
  detected_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_lpat_agent_status ON learning_patterns(agent_id, status);

CREATE TABLE IF NOT EXISTS learning_proposals (
  id TEXT PRIMARY KEY,
  agent_id TEXT NOT NULL,
  pattern_id TEXT NOT NULL DEFAULT '',
  title TEXT NOT NULL DEFAULT '',
  content TEXT NOT NULL DEFAULT '',
  kind TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'draft',
  validated_at TEXT,
  approved_by TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_lprop_agent_status ON learning_proposals(agent_id, status);
