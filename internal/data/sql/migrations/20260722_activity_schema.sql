-- Activity-First architecture: activities table for projected Activity lifecycle records.
-- See docs/reports/2026-06-13-activity-first-restructure-optimized-proposal.md

CREATE TABLE IF NOT EXISTS activities (
  id TEXT PRIMARY KEY,
  kind TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'pending',
  session_id TEXT NOT NULL DEFAULT '',
  turn_id TEXT NOT NULL DEFAULT '',
  parent_activity_id TEXT NOT NULL DEFAULT '',
  timestamp TEXT NOT NULL DEFAULT '',
  duration_ms INTEGER NOT NULL DEFAULT 0,
  content TEXT NOT NULL DEFAULT '',
  reasoning TEXT NOT NULL DEFAULT '',
  tool_name TEXT NOT NULL DEFAULT '',
  tool_call_id TEXT NOT NULL DEFAULT '',
  tool_arguments TEXT NOT NULL DEFAULT '',
  tool_result TEXT NOT NULL DEFAULT '',
  tool_duration_ms INTEGER NOT NULL DEFAULT 0,
  tool_error_code TEXT NOT NULL DEFAULT '',
  child_board_id TEXT NOT NULL DEFAULT '',
  spirit_session_id TEXT NOT NULL DEFAULT '',
  team_id TEXT NOT NULL DEFAULT '',
  dag_node_id TEXT NOT NULL DEFAULT '',
  depends_on TEXT NOT NULL DEFAULT '[]',
  agent_key TEXT NOT NULL DEFAULT '',
  agent_name TEXT NOT NULL DEFAULT '',
  collapsed INTEGER NOT NULL DEFAULT 0,
  label TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_activities_session_turn ON activities(session_id, turn_id);
CREATE INDEX IF NOT EXISTS idx_activities_parent ON activities(parent_activity_id);
CREATE INDEX IF NOT EXISTS idx_activities_spirit_session ON activities(spirit_session_id);
CREATE INDEX IF NOT EXISTS idx_activities_team ON activities(team_id);
