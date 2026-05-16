-- ============================================================
-- Memory L1: 工作记忆
-- 表: memory_l1_tasks, memory_l1_fields, memory_l1_field_history,
--     memory_l1_schemas
-- ============================================================

CREATE TABLE IF NOT EXISTS memory_l1_tasks (
  id TEXT PRIMARY KEY,
  session_id TEXT NOT NULL,
  run_id TEXT NOT NULL DEFAULT '',
  team_id TEXT NOT NULL DEFAULT '',
  agent_id TEXT NOT NULL DEFAULT '',
  task_key TEXT NOT NULL DEFAULT '',
  task_title TEXT NOT NULL DEFAULT '',
  task_goal TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'active',
  schema_version INTEGER NOT NULL DEFAULT 1,
  budget_tokens INTEGER NOT NULL DEFAULT 8192,
  used_tokens INTEGER NOT NULL DEFAULT 0,
  parent_task_id TEXT NOT NULL DEFAULT '',
  shared_with_json TEXT NOT NULL DEFAULT '[]',
  started_at TEXT NOT NULL,
  ended_at TEXT NOT NULL DEFAULT '',
  archived_at TEXT NOT NULL DEFAULT '',
  metadata_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE(session_id, task_key, agent_id)
);

CREATE TABLE IF NOT EXISTS memory_l1_fields (
  id TEXT PRIMARY KEY,
  task_id TEXT NOT NULL,
  session_id TEXT NOT NULL,
  agent_id TEXT NOT NULL DEFAULT '',
  field_path TEXT NOT NULL,
  field_kind TEXT NOT NULL DEFAULT 'string',
  visibility TEXT NOT NULL DEFAULT 'prompt',
  pin_to_prompt INTEGER NOT NULL DEFAULT 1,
  is_required INTEGER NOT NULL DEFAULT 0,
  value_text TEXT NOT NULL DEFAULT '',
  value_json TEXT NOT NULL DEFAULT '',
  value_ref TEXT NOT NULL DEFAULT '',
  preview TEXT NOT NULL DEFAULT '',
  token_estimate INTEGER NOT NULL DEFAULT 0,
  source TEXT NOT NULL DEFAULT 'agent',
  source_ref TEXT NOT NULL DEFAULT '',
  ttl_seconds INTEGER NOT NULL DEFAULT 0,
  expires_at TEXT NOT NULL DEFAULT '',
  revision INTEGER NOT NULL DEFAULT 1,
  last_read_at TEXT NOT NULL DEFAULT '',
  read_count INTEGER NOT NULL DEFAULT 0,
  metadata_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE(task_id, field_path)
);

CREATE TABLE IF NOT EXISTS memory_l1_field_history (
  id TEXT PRIMARY KEY,
  field_id TEXT NOT NULL,
  task_id TEXT NOT NULL,
  revision INTEGER NOT NULL,
  value_text TEXT NOT NULL DEFAULT '',
  value_json TEXT NOT NULL DEFAULT '',
  value_ref TEXT NOT NULL DEFAULT '',
  preview TEXT NOT NULL DEFAULT '',
  token_estimate INTEGER NOT NULL DEFAULT 0,
  changed_by TEXT NOT NULL DEFAULT '',
  change_reason TEXT NOT NULL DEFAULT '',
  diff_json TEXT NOT NULL DEFAULT '{}',
  metadata_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL,
  UNIQUE(field_id, revision)
);

CREATE TABLE IF NOT EXISTS memory_l1_schemas (
  id TEXT PRIMARY KEY,
  scope_type TEXT NOT NULL,
  scope_id TEXT NOT NULL DEFAULT '',
  schema_key TEXT NOT NULL,
  schema_version INTEGER NOT NULL DEFAULT 1,
  schema_json TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  enabled INTEGER NOT NULL DEFAULT 1,
  metadata_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE(scope_type, scope_id, schema_key, schema_version)
);
