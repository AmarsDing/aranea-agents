-- ============================================================
-- Session 相关表: sessions, messages, session_summaries, chat_attachments,
--                teams, team_runs, team_run_steps
-- ============================================================

CREATE TABLE IF NOT EXISTS sessions (
  id TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL DEFAULT '',
  user_id TEXT NOT NULL DEFAULT '',
  owner_type TEXT NOT NULL DEFAULT 'agent',
  agent_id TEXT NOT NULL DEFAULT '',
  team_id TEXT NOT NULL DEFAULT '',
  title TEXT NOT NULL DEFAULT '',
  summary TEXT NOT NULL DEFAULT '',
  tags_json TEXT NOT NULL DEFAULT '[]',
  dialog_mode TEXT NOT NULL DEFAULT '',
  default_provider TEXT NOT NULL DEFAULT '',
  default_model TEXT NOT NULL DEFAULT '',
  default_context_window_tokens INTEGER NOT NULL DEFAULT 0,
  last_provider TEXT NOT NULL DEFAULT '',
  last_model TEXT NOT NULL DEFAULT '',
  last_context_window_tokens INTEGER NOT NULL DEFAULT 0,
  status TEXT NOT NULL DEFAULT 'active',
  visibility TEXT NOT NULL DEFAULT 'private',
  message_count INTEGER NOT NULL DEFAULT 0,
  run_count INTEGER NOT NULL DEFAULT 0,
  model_call_count INTEGER NOT NULL DEFAULT 0,
  tool_call_count INTEGER NOT NULL DEFAULT 0,
  skill_call_count INTEGER NOT NULL DEFAULT 0,
  mcp_call_count INTEGER NOT NULL DEFAULT 0,
  input_tokens INTEGER NOT NULL DEFAULT 0,
  output_tokens INTEGER NOT NULL DEFAULT 0,
  total_tokens INTEGER NOT NULL DEFAULT 0,
  total_cost_micro_usd INTEGER NOT NULL DEFAULT 0,
  avg_latency_ms REAL NOT NULL DEFAULT 0,
  error_count INTEGER NOT NULL DEFAULT 0,
  context_used_tokens INTEGER NOT NULL DEFAULT 0,
  context_used_ratio REAL NOT NULL DEFAULT 0,
  max_context_used_ratio REAL NOT NULL DEFAULT 0,
  context_status TEXT NOT NULL DEFAULT 'normal',
  first_message_at TEXT NOT NULL DEFAULT '',
  last_message_at TEXT NOT NULL DEFAULT '',
  last_run_at TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL DEFAULT '',
  archived_at TEXT NOT NULL DEFAULT '',
  deleted_at TEXT NOT NULL DEFAULT '',
  runner_snapshot_json TEXT NOT NULL DEFAULT '',
  metadata_json TEXT NOT NULL DEFAULT '{}',
  session_revision INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS messages (
  id TEXT PRIMARY KEY,
  session_id TEXT NOT NULL DEFAULT '',
  parent_message_id TEXT NOT NULL DEFAULT '',
  turn_index INTEGER NOT NULL DEFAULT 0,
  role TEXT NOT NULL DEFAULT '',
  content_markdown TEXT NOT NULL DEFAULT '',
  model_name TEXT NOT NULL DEFAULT '',
  token_in INTEGER NOT NULL DEFAULT 0,
  token_out INTEGER NOT NULL DEFAULT 0,
  latency_ms INTEGER NOT NULL DEFAULT 0,
  status TEXT NOT NULL DEFAULT 'ok',
  attachments_count INTEGER NOT NULL DEFAULT 0,
  options_json TEXT NOT NULL DEFAULT '',
  error_message TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS session_summaries (
  id TEXT PRIMARY KEY,
  session_id TEXT NOT NULL,
  summary_markdown TEXT NOT NULL,
  from_turn INTEGER NOT NULL DEFAULT 0,
  to_turn INTEGER NOT NULL DEFAULT 0,
  token_estimate INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS chat_attachments (
  id TEXT PRIMARY KEY,
  session_id TEXT NOT NULL,
  message_id TEXT NOT NULL DEFAULT '',
  file_name TEXT NOT NULL,
  mime_type TEXT NOT NULL DEFAULT '',
  size_bytes INTEGER NOT NULL DEFAULT 0,
  storage_key TEXT NOT NULL DEFAULT '',
  checksum TEXT NOT NULL DEFAULT '',
  upload_status TEXT NOT NULL DEFAULT 'pending',
  created_at TEXT NOT NULL,
  deleted_at TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS teams (
  id TEXT PRIMARY KEY,
  team_key TEXT NOT NULL UNIQUE,
  display_name TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'active',
  is_default INTEGER NOT NULL DEFAULT 0,
  definition_json TEXT NOT NULL DEFAULT '',
  adk_app_name TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL DEFAULT '',
  deleted_at TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS team_runs (
  id TEXT PRIMARY KEY,
  team_id TEXT NOT NULL DEFAULT '',
  session_id TEXT NOT NULL DEFAULT '',
  message_id TEXT NOT NULL DEFAULT '',
  mode TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'running',
  input_preview TEXT NOT NULL DEFAULT '',
  output_preview TEXT NOT NULL DEFAULT '',
  token_in INTEGER NOT NULL DEFAULT 0,
  token_out INTEGER NOT NULL DEFAULT 0,
  cost_micro_usd INTEGER NOT NULL DEFAULT 0,
  duration_ms INTEGER NOT NULL DEFAULT 0,
  error_message TEXT NOT NULL DEFAULT '',
  topology_json TEXT NOT NULL DEFAULT '{}',
  graph_execution_id TEXT NOT NULL DEFAULT '',
  definition_snapshot_json TEXT NOT NULL DEFAULT '',
  trace_id TEXT NOT NULL DEFAULT '',
  started_at TEXT NOT NULL DEFAULT '',
  finished_at TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS team_run_steps (
  id TEXT PRIMARY KEY,
  run_id TEXT NOT NULL DEFAULT '',
  team_id TEXT NOT NULL DEFAULT '',
  agent_id TEXT NOT NULL DEFAULT '',
  agent_key TEXT NOT NULL DEFAULT '',
  agent_name TEXT NOT NULL DEFAULT '',
  role TEXT NOT NULL DEFAULT '',
  sort_order INTEGER NOT NULL DEFAULT 0,
  status TEXT NOT NULL DEFAULT 'success',
  input_preview TEXT NOT NULL DEFAULT '',
  output_preview TEXT NOT NULL DEFAULT '',
  token_in INTEGER NOT NULL DEFAULT 0,
  token_out INTEGER NOT NULL DEFAULT 0,
  cost_micro_usd INTEGER NOT NULL DEFAULT 0,
  duration_ms INTEGER NOT NULL DEFAULT 0,
  error_message TEXT NOT NULL DEFAULT '',
  started_at TEXT NOT NULL DEFAULT '',
  finished_at TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT '',
  tool_call_count INTEGER NOT NULL DEFAULT 0
);
-- 已有库仅缺列时执行 docs/sql/03_session_team_run_steps_tool_call_count.sql
