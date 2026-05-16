-- ============================================================
-- Skill 相关表: skill, skill_version, skill_invocation, mcp_server
-- ============================================================

CREATE TABLE IF NOT EXISTS skill (
  id TEXT PRIMARY KEY,
  skill_key TEXT NOT NULL UNIQUE,
  name TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'active',
  enabled INTEGER NOT NULL DEFAULT 1,
  sort_order INTEGER NOT NULL DEFAULT 0,
  config_json TEXT NOT NULL DEFAULT '{}',
  metadata_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL DEFAULT '',
  deleted_at TEXT NOT NULL DEFAULT '',
  parent_id TEXT NOT NULL DEFAULT '',
  level TEXT NOT NULL DEFAULT '',
  agent_id TEXT NOT NULL DEFAULT '',
  provider TEXT NOT NULL DEFAULT '',
  model TEXT NOT NULL DEFAULT '',
  kind TEXT NOT NULL DEFAULT 'markdown',
  risk_level TEXT NOT NULL DEFAULT 'low',
  entry_path TEXT NOT NULL DEFAULT 'SKILL.md',
  filesystem_missing INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS skill_version (
  id TEXT PRIMARY KEY,
  skill_id TEXT NOT NULL DEFAULT '',
  version TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'active',
  content_markdown TEXT NOT NULL DEFAULT '',
  metadata_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL DEFAULT '',
  manifest_json TEXT NOT NULL DEFAULT '{}',
  published_at TEXT NOT NULL DEFAULT '',
  validation_status TEXT NOT NULL DEFAULT '',
  UNIQUE(skill_id, version)
);

CREATE TABLE IF NOT EXISTS skill_invocation (
  id TEXT PRIMARY KEY,
  skill_id TEXT NOT NULL DEFAULT '',
  agent_id TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'pending',
  input_json TEXT NOT NULL DEFAULT '{}',
  output_json TEXT NOT NULL DEFAULT '{}',
  error_message TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL DEFAULT '',
  skill_version TEXT NOT NULL DEFAULT '',
  user_id TEXT NOT NULL DEFAULT '',
  session_id TEXT NOT NULL DEFAULT '',
  duration_ms INTEGER NOT NULL DEFAULT 0,
  started_at TEXT NOT NULL DEFAULT '',
  ended_at TEXT NOT NULL DEFAULT '',
  input_preview TEXT NOT NULL DEFAULT '',
  input_hash TEXT NOT NULL DEFAULT '',
  output_preview TEXT NOT NULL DEFAULT '',
  error_code TEXT NOT NULL DEFAULT '',
  source TEXT NOT NULL DEFAULT 'runtime',
  activation_id TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS mcp_server (
  id TEXT PRIMARY KEY,
  server_key TEXT NOT NULL UNIQUE,
  name TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'active',
  enabled INTEGER NOT NULL DEFAULT 1,
  sort_order INTEGER NOT NULL DEFAULT 0,
  config_json TEXT NOT NULL DEFAULT '',
  metadata_json TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL DEFAULT '',
  deleted_at TEXT NOT NULL DEFAULT ''
);
