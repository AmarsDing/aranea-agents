-- ============================================================
-- Evolution 相关表: evolution_suggestions, agent_evolution_events,
--                  agent_evolution_proposals, agent_skill_stats
-- ============================================================

CREATE TABLE IF NOT EXISTS evolution_suggestions (
  id TEXT PRIMARY KEY,
  agent_id TEXT NOT NULL DEFAULT '',
  type TEXT NOT NULL DEFAULT '',
  title TEXT NOT NULL DEFAULT '',
  content TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'pending',
  diff_preview TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT '',
  applied_at TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS agent_evolution_events (
  id TEXT PRIMARY KEY,
  agent_id TEXT NOT NULL,
  workspace_id TEXT NOT NULL DEFAULT '',
  event_kind TEXT NOT NULL,
  target_field TEXT NOT NULL DEFAULT '',
  before_json TEXT NOT NULL DEFAULT '',
  after_json TEXT NOT NULL DEFAULT '',
  diff_json TEXT NOT NULL DEFAULT '{}',
  trigger_kind TEXT NOT NULL,
  trigger_source TEXT NOT NULL DEFAULT '',
  evidence_json TEXT NOT NULL DEFAULT '[]',
  reason TEXT NOT NULL DEFAULT '',
  applied INTEGER NOT NULL DEFAULT 1,
  reverted INTEGER NOT NULL DEFAULT 0,
  reverted_by_event_id TEXT NOT NULL DEFAULT '',
  metadata_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL,
  applied_at TEXT NOT NULL DEFAULT '',
  reverted_at TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS agent_evolution_proposals (
  id TEXT PRIMARY KEY,
  agent_id TEXT NOT NULL,
  workspace_id TEXT NOT NULL DEFAULT '',
  proposal_kind TEXT NOT NULL,
  target_field TEXT NOT NULL DEFAULT '',
  proposed_value_json TEXT NOT NULL DEFAULT '',
  current_value_json TEXT NOT NULL DEFAULT '',
  diff_json TEXT NOT NULL DEFAULT '{}',
  rationale TEXT NOT NULL DEFAULT '',
  evidence_json TEXT NOT NULL DEFAULT '[]',
  expected_impact TEXT NOT NULL DEFAULT '',
  risk_level TEXT NOT NULL DEFAULT 'low',
  approval_required INTEGER NOT NULL DEFAULT 0,
  status TEXT NOT NULL DEFAULT 'pending',
  reviewed_by TEXT NOT NULL DEFAULT '',
  reviewed_at TEXT NOT NULL DEFAULT '',
  applied_event_id TEXT NOT NULL DEFAULT '',
  expires_at TEXT NOT NULL DEFAULT '',
  source TEXT NOT NULL,
  metadata_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS agent_skill_stats (
  agent_id TEXT NOT NULL,
  scope TEXT NOT NULL DEFAULT 'overall',
  scope_value TEXT NOT NULL DEFAULT '',
  tool_key TEXT NOT NULL,
  invocations INTEGER NOT NULL DEFAULT 0,
  successes INTEGER NOT NULL DEFAULT 0,
  failures INTEGER NOT NULL DEFAULT 0,
  user_overrides INTEGER NOT NULL DEFAULT 0,
  avg_latency_ms REAL NOT NULL DEFAULT 0,
  avg_tokens REAL NOT NULL DEFAULT 0,
  preference_score REAL NOT NULL DEFAULT 0.5,
  last_used_at TEXT NOT NULL DEFAULT '',
  metadata_json TEXT NOT NULL DEFAULT '{}',
  updated_at TEXT NOT NULL,
  PRIMARY KEY (agent_id, scope, scope_value, tool_key)
);
