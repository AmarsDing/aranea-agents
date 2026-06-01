CREATE TABLE IF NOT EXISTS session_summaries (
  id TEXT PRIMARY KEY,
  session_id TEXT NOT NULL,
  summary_markdown TEXT NOT NULL,
  from_turn INTEGER NOT NULL DEFAULT 0,
  to_turn INTEGER NOT NULL DEFAULT 0,
  token_estimate INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS memory_l0_assembly_snapshots (
  id TEXT PRIMARY KEY,
  session_id TEXT NOT NULL,
  run_id TEXT NOT NULL DEFAULT '',
  turn_id TEXT NOT NULL DEFAULT '',
  span_id TEXT NOT NULL DEFAULT '',
  agent_id TEXT NOT NULL DEFAULT '',
  team_id TEXT NOT NULL DEFAULT '',
  provider TEXT NOT NULL DEFAULT '',
  model TEXT NOT NULL DEFAULT '',
  context_window_tokens INTEGER NOT NULL DEFAULT 0,
  budget_tokens INTEGER NOT NULL DEFAULT 0,
  recent_window_turns INTEGER NOT NULL DEFAULT 0,
  recent_window_tokens INTEGER NOT NULL DEFAULT 0,
  summary_token_estimate INTEGER NOT NULL DEFAULT 0,
  l1_field_count INTEGER NOT NULL DEFAULT 0,
  l1_token_estimate INTEGER NOT NULL DEFAULT 0,
  l3_chunk_count INTEGER NOT NULL DEFAULT 0,
  l3_token_estimate INTEGER NOT NULL DEFAULT 0,
  l4_path_count INTEGER NOT NULL DEFAULT 0,
  l4_token_estimate INTEGER NOT NULL DEFAULT 0,
  prompt_token_estimate INTEGER NOT NULL DEFAULT 0,
  prompt_token_actual INTEGER NOT NULL DEFAULT 0,
  used_ratio REAL NOT NULL DEFAULT 0,
  truncate_strategy TEXT NOT NULL DEFAULT '',
  truncated_message_count INTEGER NOT NULL DEFAULT 0,
  summarized_turn_from INTEGER NOT NULL DEFAULT 0,
  summarized_turn_to INTEGER NOT NULL DEFAULT 0,
  segments_json TEXT NOT NULL DEFAULT '[]',
  warning_codes_json TEXT NOT NULL DEFAULT '[]',
  metadata_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS memory_items (
  id TEXT PRIMARY KEY,
  scope_type TEXT NOT NULL,
  scope_id TEXT NOT NULL,
  scope_subtype TEXT NOT NULL DEFAULT '',
  fact_id TEXT NOT NULL DEFAULT '',
  content TEXT NOT NULL,
  source_session_id TEXT NOT NULL DEFAULT '',
  source_message_id TEXT NOT NULL DEFAULT '',
  importance REAL NOT NULL DEFAULT 0,
  metadata_json TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS audit_logs (
  id TEXT PRIMARY KEY,
  action TEXT NOT NULL,
  resource TEXT NOT NULL,
  resource_id TEXT NOT NULL,
  request_id TEXT NOT NULL,
  detail TEXT NOT NULL,
  created_at TEXT NOT NULL,
  actor TEXT NOT NULL DEFAULT '',
  ip TEXT NOT NULL DEFAULT '',
  user_agent TEXT NOT NULL DEFAULT '',
  severity TEXT NOT NULL DEFAULT '',
  metadata_json TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS avatar_assets (
  id TEXT PRIMARY KEY,
  asset_key TEXT NOT NULL UNIQUE,
  name TEXT NOT NULL,
  image_data BLOB NOT NULL DEFAULT X'',
  thumbnail_data BLOB,
  mime_type TEXT NOT NULL DEFAULT 'image/png',
  workspace_id TEXT NOT NULL DEFAULT '',
  owner_user_id TEXT NOT NULL DEFAULT '',
  source TEXT NOT NULL DEFAULT 'system',
  is_system INTEGER NOT NULL DEFAULT 0,
  file_size_bytes INTEGER NOT NULL DEFAULT 0,
  width_px INTEGER NOT NULL DEFAULT 0,
  height_px INTEGER NOT NULL DEFAULT 0,
  description TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'active',
  enabled INTEGER NOT NULL DEFAULT 1,
  sort_order INTEGER NOT NULL DEFAULT 0,
  config_json TEXT NOT NULL DEFAULT '',
  metadata_json TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  deleted_at TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS agent_category_nodes (
  id TEXT PRIMARY KEY,
  category_key TEXT NOT NULL UNIQUE,
  name TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'active',
  enabled INTEGER NOT NULL DEFAULT 1,
  sort_order INTEGER NOT NULL DEFAULT 0,
  parent_id TEXT NOT NULL DEFAULT '',
  level TEXT NOT NULL DEFAULT '',
  workspace_id TEXT NOT NULL DEFAULT '',
  owner_user_id TEXT NOT NULL DEFAULT '',
  is_system INTEGER NOT NULL DEFAULT 0,
  config_json TEXT NOT NULL DEFAULT '',
  metadata_json TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  deleted_at TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS llm_provider_models (
  id TEXT PRIMARY KEY,
  model_key TEXT NOT NULL UNIQUE,
  name TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'active',
  enabled INTEGER NOT NULL DEFAULT 1,
  sort_order INTEGER NOT NULL DEFAULT 0,
  provider TEXT NOT NULL DEFAULT '',
  model TEXT NOT NULL DEFAULT '',
  config_json TEXT NOT NULL DEFAULT '',
  metadata_json TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  deleted_at TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS hooks (
  id TEXT PRIMARY KEY,
  hook_key TEXT NOT NULL UNIQUE,
  name TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'active',
  enabled INTEGER NOT NULL DEFAULT 1,
  sort_order INTEGER NOT NULL DEFAULT 0,
  config_json TEXT NOT NULL DEFAULT '',
  metadata_json TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  deleted_at TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS plugins (
  id TEXT PRIMARY KEY,
  plugin_key TEXT NOT NULL UNIQUE,
  name TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  category TEXT NOT NULL DEFAULT '',
  risk_level TEXT NOT NULL DEFAULT 'low',
  status TEXT NOT NULL DEFAULT 'active',
  enabled INTEGER NOT NULL DEFAULT 0,
  scope TEXT NOT NULL DEFAULT 'global',
  callback_points_json TEXT NOT NULL DEFAULT '[]',
  sort_order INTEGER NOT NULL DEFAULT 0,
  config_schema_json TEXT NOT NULL DEFAULT '{}',
  config_json TEXT NOT NULL DEFAULT '{}',
  default_config_json TEXT NOT NULL DEFAULT '{}',
  invoke_count INTEGER NOT NULL DEFAULT 0,
  block_count INTEGER NOT NULL DEFAULT 0,
  error_count INTEGER NOT NULL DEFAULT 0,
  last_invoked_at TEXT NOT NULL DEFAULT '',
  last_status TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  deleted_at TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS hook_agents (
  id TEXT PRIMARY KEY,
  hook_id TEXT NOT NULL,
  agent_id TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'active',
  enabled INTEGER NOT NULL DEFAULT 1,
  config_json TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  deleted_at TEXT NOT NULL DEFAULT '',
  UNIQUE(hook_id, agent_id)
);

CREATE TABLE IF NOT EXISTS channel (
  id TEXT PRIMARY KEY,
  channel_key TEXT NOT NULL UNIQUE,
  name TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'active',
  enabled INTEGER NOT NULL DEFAULT 1,
  sort_order INTEGER NOT NULL DEFAULT 0,
  config_json TEXT NOT NULL DEFAULT '',
  metadata_json TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  deleted_at TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS channel_credential (
  id TEXT PRIMARY KEY,
  channel_id TEXT NOT NULL,
  credential_key TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'active',
  secret_ref TEXT NOT NULL DEFAULT '',
  metadata_json TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  deleted_at TEXT NOT NULL DEFAULT '',
  UNIQUE(channel_id, credential_key)
);

CREATE TABLE IF NOT EXISTS channel_delivery (
  id TEXT PRIMARY KEY,
  channel_id TEXT NOT NULL,
  agent_id TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'pending',
  payload_json TEXT NOT NULL DEFAULT '',
  error_message TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
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
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  deleted_at TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS skill (
  id TEXT PRIMARY KEY,
  skill_key TEXT NOT NULL UNIQUE,
  name TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'active',
  enabled INTEGER NOT NULL DEFAULT 1,
  sort_order INTEGER NOT NULL DEFAULT 0,
  config_json TEXT NOT NULL DEFAULT '',
  metadata_json TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  deleted_at TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS skill_version (
  id TEXT PRIMARY KEY,
  skill_id TEXT NOT NULL,
  version TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'active',
  content_markdown TEXT NOT NULL DEFAULT '',
  metadata_json TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE(skill_id, version)
);

CREATE TABLE IF NOT EXISTS skill_invocation (
  id TEXT PRIMARY KEY,
  skill_id TEXT NOT NULL,
  agent_id TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'pending',
  input_json TEXT NOT NULL DEFAULT '',
  output_json TEXT NOT NULL DEFAULT '',
  error_message TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS cron_task (
  id TEXT PRIMARY KEY,
  task_key TEXT NOT NULL UNIQUE,
  name TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'active',
  enabled INTEGER NOT NULL DEFAULT 1,
  sort_order INTEGER NOT NULL DEFAULT 0,
  agent_id TEXT NOT NULL DEFAULT '',
  config_json TEXT NOT NULL DEFAULT '',
  metadata_json TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  deleted_at TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS cron_task_run (
  id TEXT PRIMARY KEY,
  task_id TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'pending',
  started_at TEXT NOT NULL DEFAULT '',
  finished_at TEXT NOT NULL DEFAULT '',
  output_json TEXT NOT NULL DEFAULT '',
  error_message TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS monitor_events (
  id TEXT PRIMARY KEY,
  event_key TEXT NOT NULL DEFAULT '',
  name TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'ok',
  metadata_json TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  deleted_at TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS monitor_traces (
  id TEXT PRIMARY KEY,
  trace_key TEXT NOT NULL DEFAULT '',
  name TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'ok',
  metadata_json TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  deleted_at TEXT NOT NULL DEFAULT ''
);

-- Usage + tooling (sync with docs/sql/08_usage.sql); required before downstream indexes referencing these tables.
CREATE TABLE IF NOT EXISTS model_token_usage_events (
  id TEXT PRIMARY KEY,
  occurred_at TEXT NOT NULL,
  date_key TEXT NOT NULL,
  hour_key TEXT NOT NULL,
  workspace_id TEXT NOT NULL DEFAULT '',
  user_id TEXT NOT NULL DEFAULT '',
  team_id TEXT NOT NULL DEFAULT '',
  agent_id TEXT NOT NULL DEFAULT '',
  agent_key TEXT NOT NULL DEFAULT '',
  session_id TEXT NOT NULL DEFAULT '',
  message_id TEXT NOT NULL DEFAULT '',
  request_id TEXT NOT NULL DEFAULT '',
  provider_code TEXT NOT NULL DEFAULT '',
  provider_type TEXT NOT NULL DEFAULT '',
  provider_display_name TEXT NOT NULL DEFAULT '',
  model_api_id TEXT NOT NULL DEFAULT '',
  model_display_name TEXT NOT NULL DEFAULT '',
  model_category_json TEXT NOT NULL DEFAULT '[]',
  usage_kind TEXT NOT NULL DEFAULT 'chat',
  call_count INTEGER NOT NULL DEFAULT 1,
  input_tokens INTEGER NOT NULL DEFAULT 0,
  output_tokens INTEGER NOT NULL DEFAULT 0,
  cached_input_tokens INTEGER NOT NULL DEFAULT 0,
  reasoning_tokens INTEGER NOT NULL DEFAULT 0,
  embedding_tokens INTEGER NOT NULL DEFAULT 0,
  total_tokens INTEGER NOT NULL DEFAULT 0,
  input_price_micro_usd_per_1k INTEGER NOT NULL DEFAULT 0,
  output_price_micro_usd_per_1k INTEGER NOT NULL DEFAULT 0,
  cached_input_price_micro_usd_per_1k INTEGER NOT NULL DEFAULT 0,
  reasoning_price_micro_usd_per_1k INTEGER NOT NULL DEFAULT 0,
  embedding_price_micro_usd_per_1k INTEGER NOT NULL DEFAULT 0,
  input_cost_micro_usd INTEGER NOT NULL DEFAULT 0,
  output_cost_micro_usd INTEGER NOT NULL DEFAULT 0,
  cached_input_cost_micro_usd INTEGER NOT NULL DEFAULT 0,
  reasoning_cost_micro_usd INTEGER NOT NULL DEFAULT 0,
  embedding_cost_micro_usd INTEGER NOT NULL DEFAULT 0,
  total_cost_micro_usd INTEGER NOT NULL DEFAULT 0,
  latency_ms INTEGER NOT NULL DEFAULT 0,
  time_to_first_token_ms INTEGER NOT NULL DEFAULT 0,
  tokens_per_second REAL NOT NULL DEFAULT 0,
  status TEXT NOT NULL DEFAULT 'success',
  error_code TEXT NOT NULL DEFAULT '',
  error_message TEXT NOT NULL DEFAULT '',
  retry_count INTEGER NOT NULL DEFAULT 0,
  prompt_mode TEXT NOT NULL DEFAULT '',
  max_output_tokens INTEGER NOT NULL DEFAULT 0,
  context_window_k INTEGER NOT NULL DEFAULT 0,
  stream_enabled INTEGER NOT NULL DEFAULT 0,
  metadata_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS model_token_usage_daily (
  id TEXT PRIMARY KEY,
  date_key TEXT NOT NULL,
  workspace_id TEXT NOT NULL DEFAULT '',
  agent_id TEXT NOT NULL DEFAULT '',
  agent_key TEXT NOT NULL DEFAULT '',
  provider_code TEXT NOT NULL DEFAULT '',
  model_api_id TEXT NOT NULL DEFAULT '',
  usage_kind TEXT NOT NULL DEFAULT 'chat',
  call_count INTEGER NOT NULL DEFAULT 0,
  request_count INTEGER NOT NULL DEFAULT 0,
  success_count INTEGER NOT NULL DEFAULT 0,
  failed_count INTEGER NOT NULL DEFAULT 0,
  cancelled_count INTEGER NOT NULL DEFAULT 0,
  input_tokens INTEGER NOT NULL DEFAULT 0,
  output_tokens INTEGER NOT NULL DEFAULT 0,
  cached_input_tokens INTEGER NOT NULL DEFAULT 0,
  reasoning_tokens INTEGER NOT NULL DEFAULT 0,
  embedding_tokens INTEGER NOT NULL DEFAULT 0,
  total_tokens INTEGER NOT NULL DEFAULT 0,
  total_cost_micro_usd INTEGER NOT NULL DEFAULT 0,
  avg_latency_ms REAL NOT NULL DEFAULT 0,
  avg_tokens_per_second REAL NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE(date_key, workspace_id, agent_id, provider_code, model_api_id, usage_kind)
);

CREATE TABLE IF NOT EXISTS model_pricing_rules (
  id TEXT PRIMARY KEY,
  provider_code TEXT NOT NULL,
  model_api_id TEXT NOT NULL,
  currency TEXT NOT NULL DEFAULT 'USD',
  input_price_micro_usd_per_1k INTEGER NOT NULL DEFAULT 0,
  output_price_micro_usd_per_1k INTEGER NOT NULL DEFAULT 0,
  cached_input_price_micro_usd_per_1k INTEGER NOT NULL DEFAULT 0,
  reasoning_price_micro_usd_per_1k INTEGER NOT NULL DEFAULT 0,
  embedding_price_micro_usd_per_1k INTEGER NOT NULL DEFAULT 0,
  effective_from TEXT NOT NULL DEFAULT '',
  effective_to TEXT NOT NULL DEFAULT '',
  is_active INTEGER NOT NULL DEFAULT 1,
  source TEXT NOT NULL DEFAULT 'manual',
  metadata_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL DEFAULT '',
  UNIQUE(provider_code, model_api_id, effective_from)
);

CREATE TABLE IF NOT EXISTS usage_quotas (
  id TEXT PRIMARY KEY,
  scope_type TEXT NOT NULL,
  scope_id TEXT NOT NULL,
  monthly_micro_usd INTEGER NOT NULL DEFAULT 0,
  period_start TEXT NOT NULL DEFAULT '',
  period_end TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE(scope_type, scope_id)
);

CREATE TABLE IF NOT EXISTS budget_alerts (
  id TEXT PRIMARY KEY,
  scope_type TEXT NOT NULL,
  scope_id TEXT NOT NULL,
  alert_ratio REAL NOT NULL DEFAULT 0.8,
  enabled INTEGER NOT NULL DEFAULT 1,
  last_fired_at TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE(scope_type, scope_id, alert_ratio)
);

CREATE TABLE IF NOT EXISTS model_token_usage_hourly (
  id TEXT PRIMARY KEY,
  hour_key TEXT NOT NULL,
  workspace_id TEXT NOT NULL DEFAULT '',
  agent_id TEXT NOT NULL DEFAULT '',
  agent_key TEXT NOT NULL DEFAULT '',
  provider_code TEXT NOT NULL DEFAULT '',
  model_api_id TEXT NOT NULL DEFAULT '',
  usage_kind TEXT NOT NULL DEFAULT 'chat',
  call_count INTEGER NOT NULL DEFAULT 0,
  request_count INTEGER NOT NULL DEFAULT 0,
  success_count INTEGER NOT NULL DEFAULT 0,
  failed_count INTEGER NOT NULL DEFAULT 0,
  cancelled_count INTEGER NOT NULL DEFAULT 0,
  input_tokens INTEGER NOT NULL DEFAULT 0,
  output_tokens INTEGER NOT NULL DEFAULT 0,
  cached_input_tokens INTEGER NOT NULL DEFAULT 0,
  reasoning_tokens INTEGER NOT NULL DEFAULT 0,
  embedding_tokens INTEGER NOT NULL DEFAULT 0,
  total_tokens INTEGER NOT NULL DEFAULT 0,
  total_cost_micro_usd INTEGER NOT NULL DEFAULT 0,
  avg_latency_ms REAL NOT NULL DEFAULT 0,
  avg_tokens_per_second REAL NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE(hour_key, workspace_id, agent_id, provider_code, model_api_id, usage_kind)
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

CREATE TABLE IF NOT EXISTS tools (
  id TEXT PRIMARY KEY,
  tool_key TEXT NOT NULL UNIQUE,
  display_name TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  category TEXT NOT NULL DEFAULT 'system',
  source TEXT NOT NULL DEFAULT 'builtin',
  risk_level TEXT NOT NULL DEFAULT 'low',
  enabled INTEGER NOT NULL DEFAULT 1,
  readonly INTEGER NOT NULL DEFAULT 0,
  requires_confirmation INTEGER NOT NULL DEFAULT 0,
  supports_streaming INTEGER NOT NULL DEFAULT 0,
  supports_concurrency INTEGER NOT NULL DEFAULT 0,
  parameters_schema_json TEXT NOT NULL DEFAULT '{}',
  result_schema_json TEXT NOT NULL DEFAULT '{}',
  config_schema_json TEXT NOT NULL DEFAULT '{}',
  config_json TEXT NOT NULL DEFAULT '{}',
  default_config_json TEXT NOT NULL DEFAULT '{}',
  metadata_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  deleted_at TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS tool_agent_overrides (
  id TEXT PRIMARY KEY,
  tool_id TEXT NOT NULL,
  tool_key TEXT NOT NULL,
  agent_id TEXT NOT NULL,
  enabled INTEGER NOT NULL DEFAULT 1,
  mode TEXT NOT NULL DEFAULT 'inherit',
  config_override_json TEXT NOT NULL DEFAULT '{}',
  requires_confirmation INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  deleted_at TEXT NOT NULL DEFAULT '',
  UNIQUE(tool_key, agent_id)
);

CREATE TABLE IF NOT EXISTS tool_invocations (
  id TEXT PRIMARY KEY,
  request_id TEXT NOT NULL DEFAULT '',
  invocation_id TEXT NOT NULL DEFAULT '',
  tool_id TEXT NOT NULL DEFAULT '',
  tool_key TEXT NOT NULL,
  agent_id TEXT NOT NULL DEFAULT '',
  agent_key TEXT NOT NULL DEFAULT '',
  session_id TEXT NOT NULL DEFAULT '',
  message_id TEXT NOT NULL DEFAULT '',
  user_id TEXT NOT NULL DEFAULT '',
  source TEXT NOT NULL DEFAULT 'adk',
  status TEXT NOT NULL DEFAULT 'success',
  started_at TEXT NOT NULL,
  ended_at TEXT NOT NULL DEFAULT '',
  duration_ms INTEGER NOT NULL DEFAULT 0,
  input_preview TEXT NOT NULL DEFAULT '',
  input_hash TEXT NOT NULL DEFAULT '',
  output_preview TEXT NOT NULL DEFAULT '',
  output_hash TEXT NOT NULL DEFAULT '',
  error_code TEXT NOT NULL DEFAULT '',
  error_message TEXT NOT NULL DEFAULT '',
  redaction_applied INTEGER NOT NULL DEFAULT 1,
  streaming INTEGER NOT NULL DEFAULT 0,
  chunk_count INTEGER NOT NULL DEFAULT 0,
  metadata_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS tool_invocation_params (
  id TEXT PRIMARY KEY,
  invocation_id TEXT NOT NULL,
  tool_key TEXT NOT NULL,
  param_name TEXT NOT NULL,
  param_type TEXT NOT NULL DEFAULT 'string',
  value_preview TEXT NOT NULL DEFAULT '',
  value_hash TEXT NOT NULL DEFAULT '',
  value_size_bytes INTEGER NOT NULL DEFAULT 0,
  is_required INTEGER NOT NULL DEFAULT 0,
  is_sensitive INTEGER NOT NULL DEFAULT 0,
  redaction_reason TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS tool_usage_daily (
  id TEXT PRIMARY KEY,
  date_key TEXT NOT NULL,
  tool_key TEXT NOT NULL,
  agent_id TEXT NOT NULL DEFAULT '',
  call_count INTEGER NOT NULL DEFAULT 0,
  success_count INTEGER NOT NULL DEFAULT 0,
  failure_count INTEGER NOT NULL DEFAULT 0,
  blocked_count INTEGER NOT NULL DEFAULT 0,
  total_duration_ms INTEGER NOT NULL DEFAULT 0,
  avg_duration_ms REAL NOT NULL DEFAULT 0,
  p95_duration_ms REAL NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE(date_key, tool_key, agent_id)
);

CREATE TABLE IF NOT EXISTS tool_invocation_audit (
  id TEXT PRIMARY KEY,
  invocation_id TEXT NOT NULL DEFAULT '',
  tool_key TEXT NOT NULL,
  agent_id TEXT NOT NULL DEFAULT '',
  user_id TEXT NOT NULL DEFAULT '',
  session_id TEXT NOT NULL DEFAULT '',
  action TEXT NOT NULL DEFAULT 'tool.call',
  result_summary TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'success',
  source TEXT NOT NULL DEFAULT 'adk',
  created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_session_summaries_session_range ON session_summaries(session_id, to_turn);
CREATE INDEX IF NOT EXISTS idx_memory_l0_snapshots_session ON memory_l0_assembly_snapshots(session_id, created_at);
CREATE INDEX IF NOT EXISTS idx_memory_l0_snapshots_span ON memory_l0_assembly_snapshots(span_id);
CREATE INDEX IF NOT EXISTS idx_memory_l0_snapshots_agent ON memory_l0_assembly_snapshots(agent_id, created_at);
CREATE INDEX IF NOT EXISTS idx_usage_events_time ON model_token_usage_events(occurred_at);
CREATE INDEX IF NOT EXISTS idx_usage_events_date_model ON model_token_usage_events(date_key, provider_code, model_api_id);
CREATE INDEX IF NOT EXISTS idx_usage_events_agent_time ON model_token_usage_events(agent_id, occurred_at);
CREATE INDEX IF NOT EXISTS idx_usage_events_session ON model_token_usage_events(session_id);
CREATE INDEX IF NOT EXISTS idx_usage_events_status ON model_token_usage_events(status, occurred_at);
CREATE INDEX IF NOT EXISTS idx_usage_daily_date_model ON model_token_usage_daily(date_key, provider_code, model_api_id);
CREATE INDEX IF NOT EXISTS idx_usage_hourly_hour ON model_token_usage_hourly(hour_key);
CREATE INDEX IF NOT EXISTS idx_attachments_session ON chat_attachments(session_id, deleted_at);
CREATE INDEX IF NOT EXISTS idx_tools_category ON tools(category);
CREATE INDEX IF NOT EXISTS idx_tools_source ON tools(source);
CREATE INDEX IF NOT EXISTS idx_tools_enabled ON tools(enabled);
CREATE INDEX IF NOT EXISTS idx_tools_risk_level ON tools(risk_level);
CREATE INDEX IF NOT EXISTS idx_tool_agent_overrides_agent ON tool_agent_overrides(agent_id);
CREATE INDEX IF NOT EXISTS idx_tool_agent_overrides_tool ON tool_agent_overrides(tool_key);
CREATE INDEX IF NOT EXISTS idx_tool_invocation_params_invocation ON tool_invocation_params(invocation_id);
CREATE INDEX IF NOT EXISTS idx_tool_invocation_params_tool_param ON tool_invocation_params(tool_key, param_name);
CREATE INDEX IF NOT EXISTS idx_tool_invocation_audit_tool_time ON tool_invocation_audit(tool_key, created_at);
CREATE INDEX IF NOT EXISTS idx_tool_invocation_audit_agent_time ON tool_invocation_audit(agent_id, created_at);
CREATE INDEX IF NOT EXISTS idx_tool_invocation_audit_user_time ON tool_invocation_audit(user_id, created_at);
CREATE INDEX IF NOT EXISTS idx_tool_usage_daily_tool_date ON tool_usage_daily(tool_key, date_key);
CREATE INDEX IF NOT EXISTS idx_tool_usage_daily_agent_date ON tool_usage_daily(agent_id, date_key);
CREATE INDEX IF NOT EXISTS idx_memory_scope ON memory_items(scope_type, scope_id);
CREATE INDEX IF NOT EXISTS idx_hook_agents_agent ON hook_agents(agent_id, deleted_at);
CREATE INDEX IF NOT EXISTS idx_monitor_events_created ON monitor_events(created_at);
CREATE INDEX IF NOT EXISTS idx_monitor_events_key_created ON monitor_events(event_key, created_at);
CREATE INDEX IF NOT EXISTS idx_monitor_events_key_status_created ON monitor_events(event_key, status, created_at);
CREATE UNIQUE INDEX IF NOT EXISTS idx_monitor_events_runner_completion_unique ON monitor_events(event_key, json_extract(metadata_json, '$.session_id'), json_extract(metadata_json, '$.invocation_id')) WHERE event_key = 'runner.completion' AND deleted_at = '';
CREATE INDEX IF NOT EXISTS idx_monitor_traces_created ON monitor_traces(created_at);

-- L1 working memory (aranea/docs/13 memory-L1-working.md 搂3)

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

CREATE INDEX IF NOT EXISTS idx_memory_l1_tasks_session ON memory_l1_tasks(session_id, status, updated_at);
CREATE INDEX IF NOT EXISTS idx_memory_l1_tasks_agent ON memory_l1_tasks(agent_id, status);
CREATE INDEX IF NOT EXISTS idx_memory_l1_fields_task ON memory_l1_fields(task_id, visibility, pin_to_prompt);
CREATE INDEX IF NOT EXISTS idx_memory_l1_fields_session ON memory_l1_fields(session_id, updated_at);
CREATE INDEX IF NOT EXISTS idx_memory_l1_field_history_field ON memory_l1_field_history(field_id, revision DESC);

-- L2 episodic memory (aranea/docs/14 memory-L2-episodic.md 搂3)

CREATE TABLE IF NOT EXISTS memory_episodes (
  id TEXT PRIMARY KEY,
  session_id TEXT NOT NULL,
  run_id TEXT NOT NULL DEFAULT '',
  team_id TEXT NOT NULL DEFAULT '',
  agent_id TEXT NOT NULL DEFAULT '',
  l1_task_id TEXT NOT NULL DEFAULT '',
  episode_kind TEXT NOT NULL DEFAULT 'task',
  title TEXT NOT NULL,
  goal TEXT NOT NULL DEFAULT '',
  outcome TEXT NOT NULL DEFAULT '',
  outcome_summary TEXT NOT NULL DEFAULT '',
  result_preview TEXT NOT NULL DEFAULT '',
  failure_reason TEXT NOT NULL DEFAULT '',
  importance REAL NOT NULL DEFAULT 0.5,
  confidence REAL NOT NULL DEFAULT 0.7,
  user_feedback TEXT NOT NULL DEFAULT '',
  critic_score REAL NOT NULL DEFAULT -1,
  span_count INTEGER NOT NULL DEFAULT 0,
  message_count INTEGER NOT NULL DEFAULT 0,
  tool_call_count INTEGER NOT NULL DEFAULT 0,
  skill_call_count INTEGER NOT NULL DEFAULT 0,
  mcp_call_count INTEGER NOT NULL DEFAULT 0,
  total_tokens INTEGER NOT NULL DEFAULT 0,
  total_cost_micro_usd INTEGER NOT NULL DEFAULT 0,
  duration_ms INTEGER NOT NULL DEFAULT 0,
  l1_snapshot_json TEXT NOT NULL DEFAULT '{}',
  key_decisions_json TEXT NOT NULL DEFAULT '[]',
  key_artifacts_json TEXT NOT NULL DEFAULT '[]',
  embedding_status TEXT NOT NULL DEFAULT 'pending',
  embedding_model TEXT NOT NULL DEFAULT '',
  embedding_dim INTEGER NOT NULL DEFAULT 0,
  embedding_blob BLOB,
  embedding_norm REAL NOT NULL DEFAULT 0,
  consolidation_status TEXT NOT NULL DEFAULT 'pending',
  consolidated_at TEXT NOT NULL DEFAULT '',
  consolidated_l3_count INTEGER NOT NULL DEFAULT 0,
  consolidated_l4_count INTEGER NOT NULL DEFAULT 0,
  metadata_json TEXT NOT NULL DEFAULT '{}',
  started_at TEXT NOT NULL DEFAULT '',
  ended_at TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  archived_at TEXT NOT NULL DEFAULT '',
  deleted_at TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS memory_l2_index_meta (
  id TEXT PRIMARY KEY,
  episode_id TEXT NOT NULL,
  session_id TEXT NOT NULL,
  agent_id TEXT NOT NULL DEFAULT '',
  text_kind TEXT NOT NULL DEFAULT 'episode',
  text_preview TEXT NOT NULL DEFAULT '',
  token_estimate INTEGER NOT NULL DEFAULT 0,
  embedding_model TEXT NOT NULL DEFAULT '',
  embedding_dim INTEGER NOT NULL DEFAULT 0,
  embedding_blob BLOB,
  embedding_norm REAL NOT NULL DEFAULT 0,
  importance REAL NOT NULL DEFAULT 0.5,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE(episode_id, text_kind)
);

CREATE TABLE IF NOT EXISTS memory_event_marks (
  id TEXT PRIMARY KEY,
  session_id TEXT NOT NULL,
  episode_id TEXT NOT NULL DEFAULT '',
  ref_kind TEXT NOT NULL,
  ref_id TEXT NOT NULL,
  mark_type TEXT NOT NULL,
  marked_by TEXT NOT NULL DEFAULT '',
  reason TEXT NOT NULL DEFAULT '',
  weight REAL NOT NULL DEFAULT 1.0,
  metadata_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL,
  deleted_at TEXT NOT NULL DEFAULT '',
  UNIQUE(ref_kind, ref_id, mark_type, marked_by)
);

CREATE INDEX IF NOT EXISTS idx_memory_episodes_session ON memory_episodes(session_id, ended_at DESC);
CREATE INDEX IF NOT EXISTS idx_memory_episodes_agent ON memory_episodes(agent_id, importance DESC, ended_at DESC);
CREATE INDEX IF NOT EXISTS idx_memory_episodes_consolidation ON memory_episodes(consolidation_status, importance DESC, ended_at);
CREATE INDEX IF NOT EXISTS idx_memory_episodes_kind ON memory_episodes(episode_kind, ended_at DESC);
CREATE INDEX IF NOT EXISTS idx_memory_episodes_l1_task ON memory_episodes(l1_task_id);
CREATE INDEX IF NOT EXISTS idx_memory_l2_index_meta_episode ON memory_l2_index_meta(episode_id);
CREATE INDEX IF NOT EXISTS idx_memory_l2_index_meta_session_kind ON memory_l2_index_meta(session_id, text_kind);
CREATE INDEX IF NOT EXISTS idx_memory_event_marks_session ON memory_event_marks(session_id, mark_type, created_at);
CREATE INDEX IF NOT EXISTS idx_memory_event_marks_episode ON memory_event_marks(episode_id);

-- Policy audit trail (memory.design.md §2.4)

CREATE TABLE IF NOT EXISTS memory_action_log (
  id TEXT PRIMARY KEY,
  action TEXT NOT NULL,
  target_kind TEXT NOT NULL,
  target_id TEXT NOT NULL,
  reason TEXT NOT NULL DEFAULT '',
  policy_version TEXT NOT NULL DEFAULT 'consolidate_v1',
  source_event_ids_json TEXT NOT NULL DEFAULT '[]',
  metadata_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_memory_action_log_target ON memory_action_log(target_kind, target_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_memory_action_log_created ON memory_action_log(created_at DESC);

CREATE TABLE IF NOT EXISTS memory_cascade_proposals (
  id TEXT PRIMARY KEY,
  agent_id TEXT NOT NULL,
  workspace_id TEXT NOT NULL DEFAULT '',
  trigger_entity_id TEXT NOT NULL,
  trigger_entity_name TEXT NOT NULL DEFAULT '',
  trigger_attribute TEXT NOT NULL DEFAULT 'name',
  old_value TEXT NOT NULL DEFAULT '',
  new_value TEXT NOT NULL DEFAULT '',
  affected_json TEXT NOT NULL DEFAULT '[]',
  status TEXT NOT NULL DEFAULT 'pending',
  risk_level TEXT NOT NULL DEFAULT 'medium',
  rationale TEXT NOT NULL DEFAULT '',
  metadata_json TEXT NOT NULL DEFAULT '{}',
  reviewed_by TEXT NOT NULL DEFAULT '',
  reviewed_at TEXT NOT NULL DEFAULT '',
  expires_at TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_memory_cascade_proposals_agent_status ON memory_cascade_proposals(agent_id, status, created_at DESC);

-- L3 semantic memory (aranea/docs/15 memory-L3-semantic.md 搂3)

CREATE TABLE IF NOT EXISTS memory_facts (
  id TEXT PRIMARY KEY,
  scope_type TEXT NOT NULL,
  scope_id TEXT NOT NULL DEFAULT '',
  workspace_id TEXT NOT NULL DEFAULT '',
  user_id TEXT NOT NULL DEFAULT '',
  team_id TEXT NOT NULL DEFAULT '',
  agent_id TEXT NOT NULL DEFAULT '',
  statement TEXT NOT NULL,
  statement_normalized TEXT NOT NULL DEFAULT '',
  fingerprint TEXT NOT NULL DEFAULT '',
  details_markdown TEXT NOT NULL DEFAULT '',
  fact_kind TEXT NOT NULL DEFAULT 'fact',
  tags_json TEXT NOT NULL DEFAULT '[]',
  confidence REAL NOT NULL DEFAULT 0.7,
  importance REAL NOT NULL DEFAULT 0.5,
  use_count INTEGER NOT NULL DEFAULT 0,
  hit_count INTEGER NOT NULL DEFAULT 0,
  positive_feedback_count INTEGER NOT NULL DEFAULT 0,
  negative_feedback_count INTEGER NOT NULL DEFAULT 0,
  conflict_count INTEGER NOT NULL DEFAULT 0,
  source_kind TEXT NOT NULL DEFAULT 'episode',
  source_episode_id TEXT NOT NULL DEFAULT '',
  source_session_id TEXT NOT NULL DEFAULT '',
  source_message_id TEXT NOT NULL DEFAULT '',
  source_external TEXT NOT NULL DEFAULT '',
  version INTEGER NOT NULL DEFAULT 1,
  status TEXT NOT NULL DEFAULT 'active',
  superseded_by TEXT NOT NULL DEFAULT '',
  embedding_status TEXT NOT NULL DEFAULT 'pending',
  embedding_model TEXT NOT NULL DEFAULT '',
  embedding_dim INTEGER NOT NULL DEFAULT 0,
  embedding_blob BLOB,
  embedding_norm REAL NOT NULL DEFAULT 0,
  pii_flag INTEGER NOT NULL DEFAULT 0,
  redacted_statement TEXT NOT NULL DEFAULT '',
  ttl_days INTEGER NOT NULL DEFAULT 0,
  decay_factor REAL NOT NULL DEFAULT 0.98,
  next_decay_at TEXT NOT NULL DEFAULT '',
  last_used_at TEXT NOT NULL DEFAULT '',
  expires_at TEXT NOT NULL DEFAULT '',
  metadata_json TEXT NOT NULL DEFAULT '{}',
  -- MEM-OPT-01 Phase 0: external vector index (pgvector / embedding_blob) consistency tracking
  index_status TEXT NOT NULL DEFAULT 'fresh',
  index_synced_at INTEGER NOT NULL DEFAULT 0,
  index_attempts INTEGER NOT NULL DEFAULT 0,
  index_last_error TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  archived_at TEXT NOT NULL DEFAULT '',
  deleted_at TEXT NOT NULL DEFAULT '',
  UNIQUE(scope_type, scope_id, fingerprint)
);

CREATE TABLE IF NOT EXISTS memory_fact_versions (
  id TEXT PRIMARY KEY,
  fact_id TEXT NOT NULL,
  version INTEGER NOT NULL,
  statement TEXT NOT NULL,
  details_markdown TEXT NOT NULL DEFAULT '',
  tags_json TEXT NOT NULL DEFAULT '[]',
  confidence REAL NOT NULL DEFAULT 0.7,
  status TEXT NOT NULL DEFAULT 'active',
  changed_by TEXT NOT NULL DEFAULT '',
  change_reason TEXT NOT NULL DEFAULT '',
  diff_json TEXT NOT NULL DEFAULT '{}',
  metadata_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL,
  UNIQUE(fact_id, version)
);

CREATE TABLE IF NOT EXISTS memory_fact_feedback (
  id TEXT PRIMARY KEY,
  fact_id TEXT NOT NULL,
  session_id TEXT NOT NULL DEFAULT '',
  agent_id TEXT NOT NULL DEFAULT '',
  feedback_type TEXT NOT NULL,
  source TEXT NOT NULL,
  weight REAL NOT NULL DEFAULT 1.0,
  comment TEXT NOT NULL DEFAULT '',
  metadata_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS memory_fact_conflicts (
  id TEXT PRIMARY KEY,
  fact_a_id TEXT NOT NULL,
  fact_b_id TEXT NOT NULL,
  scope_type TEXT NOT NULL,
  scope_id TEXT NOT NULL DEFAULT '',
  conflict_kind TEXT NOT NULL,
  similarity REAL NOT NULL DEFAULT 0,
  status TEXT NOT NULL DEFAULT 'open',
  detected_by TEXT NOT NULL DEFAULT '',
  resolution TEXT NOT NULL DEFAULT '',
  resolved_by TEXT NOT NULL DEFAULT '',
  resolved_at TEXT NOT NULL DEFAULT '',
  metadata_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE(fact_a_id, fact_b_id)
);

CREATE TABLE IF NOT EXISTS memory_fact_index (
  fact_id TEXT PRIMARY KEY,
  scope_type TEXT NOT NULL,
  scope_id TEXT NOT NULL DEFAULT '',
  embedding_model TEXT NOT NULL DEFAULT '',
  embedding_dim INTEGER NOT NULL DEFAULT 0,
  embedding_blob BLOB,
  embedding_norm REAL NOT NULL DEFAULT 0,
  importance REAL NOT NULL DEFAULT 0.5,
  confidence REAL NOT NULL DEFAULT 0.7,
  updated_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_memory_facts_scope_status ON memory_facts(scope_type, scope_id, status, updated_at);
CREATE INDEX IF NOT EXISTS idx_memory_facts_workspace ON memory_facts(workspace_id, status, updated_at);
CREATE INDEX IF NOT EXISTS idx_memory_facts_agent ON memory_facts(agent_id, status, last_used_at);
CREATE INDEX IF NOT EXISTS idx_memory_facts_decay ON memory_facts(status, next_decay_at);
CREATE INDEX IF NOT EXISTS idx_memory_facts_kind ON memory_facts(fact_kind, scope_type, scope_id);
-- MEM-OPT-01 Phase 0: reconciler can quickly find stale rows that need re-sync
CREATE INDEX IF NOT EXISTS idx_memory_facts_index_status ON memory_facts(index_status, index_synced_at);
CREATE INDEX IF NOT EXISTS idx_memory_fact_versions_fact ON memory_fact_versions(fact_id, version DESC);
CREATE INDEX IF NOT EXISTS idx_memory_fact_feedback_fact ON memory_fact_feedback(fact_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_memory_fact_feedback_session ON memory_fact_feedback(session_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_memory_fact_conflicts_status ON memory_fact_conflicts(status, created_at);
CREATE INDEX IF NOT EXISTS idx_memory_fact_index_scope ON memory_fact_index(scope_type, scope_id);

-- L4 persistent / evolutionary memory (aranea/docs/16 memory-L4-persistent.md 搂3)

CREATE TABLE IF NOT EXISTS memory_entities (
  id TEXT PRIMARY KEY,
  scope_type TEXT NOT NULL,
  scope_id TEXT NOT NULL DEFAULT '',
  workspace_id TEXT NOT NULL DEFAULT '',
  user_id TEXT NOT NULL DEFAULT '',
  entity_type TEXT NOT NULL,
  name TEXT NOT NULL,
  name_normalized TEXT NOT NULL DEFAULT '',
  aliases_json TEXT NOT NULL DEFAULT '[]',
  description TEXT NOT NULL DEFAULT '',
  attributes_json TEXT NOT NULL DEFAULT '{}',
  importance REAL NOT NULL DEFAULT 0.5,
  confidence REAL NOT NULL DEFAULT 0.7,
  use_count INTEGER NOT NULL DEFAULT 0,
  source_kind TEXT NOT NULL DEFAULT 'extracted',
  embedding_status TEXT NOT NULL DEFAULT 'pending',
  embedding_model TEXT NOT NULL DEFAULT '',
  embedding_dim INTEGER NOT NULL DEFAULT 0,
  embedding_blob BLOB,
  embedding_norm REAL NOT NULL DEFAULT 0,
  status TEXT NOT NULL DEFAULT 'active',
  merged_into TEXT NOT NULL DEFAULT '',
  metadata_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  archived_at TEXT NOT NULL DEFAULT '',
  deleted_at TEXT NOT NULL DEFAULT '',
  UNIQUE(scope_type, scope_id, entity_type, name_normalized)
);

CREATE TABLE IF NOT EXISTS memory_relations (
  id TEXT PRIMARY KEY,
  scope_type TEXT NOT NULL,
  scope_id TEXT NOT NULL DEFAULT '',
  workspace_id TEXT NOT NULL DEFAULT '',
  source_id TEXT NOT NULL,
  target_id TEXT NOT NULL,
  relation_type TEXT NOT NULL,
  bidirectional INTEGER NOT NULL DEFAULT 0,
  weight REAL NOT NULL DEFAULT 1.0,
  confidence REAL NOT NULL DEFAULT 0.7,
  importance REAL NOT NULL DEFAULT 0.5,
  use_count INTEGER NOT NULL DEFAULT 0,
  attributes_json TEXT NOT NULL DEFAULT '{}',
  evidence_json TEXT NOT NULL DEFAULT '[]',
  status TEXT NOT NULL DEFAULT 'active',
  source_kind TEXT NOT NULL DEFAULT 'extracted',
  metadata_json TEXT NOT NULL DEFAULT '{}',
  valid_from TEXT NOT NULL DEFAULT '',
  valid_to TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  archived_at TEXT NOT NULL DEFAULT '',
  deleted_at TEXT NOT NULL DEFAULT '',
  UNIQUE(scope_type, scope_id, source_id, target_id, relation_type)
);

CREATE TABLE IF NOT EXISTS memory_entity_facts (
  entity_id TEXT NOT NULL,
  fact_id TEXT NOT NULL,
  weight REAL NOT NULL DEFAULT 1.0,
  created_at TEXT NOT NULL,
  PRIMARY KEY (entity_id, fact_id)
);

CREATE TABLE IF NOT EXISTS memory_entity_versions (
  id TEXT PRIMARY KEY,
  entity_id TEXT NOT NULL,
  version INTEGER NOT NULL,
  snapshot_json TEXT NOT NULL,
  changed_by TEXT NOT NULL DEFAULT '',
  change_reason TEXT NOT NULL DEFAULT '',
  diff_json TEXT NOT NULL DEFAULT '{}',
  metadata_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL,
  UNIQUE(entity_id, version)
);

CREATE TABLE IF NOT EXISTS agent_identity (
  agent_id TEXT PRIMARY KEY,
  persona TEXT NOT NULL DEFAULT '',
  values_json TEXT NOT NULL DEFAULT '[]',
  tone TEXT NOT NULL DEFAULT '',
  domains_json TEXT NOT NULL DEFAULT '[]',
  user_expectations TEXT NOT NULL DEFAULT '',
  current_phase TEXT NOT NULL DEFAULT 'cold-start',
  metadata_json TEXT NOT NULL DEFAULT '{}',
  version INTEGER NOT NULL DEFAULT 1,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS agent_strategy_profile (
  agent_id TEXT PRIMARY KEY,
  exploration REAL NOT NULL DEFAULT 0.5,
  conciseness REAL NOT NULL DEFAULT 0.5,
  caution REAL NOT NULL DEFAULT 0.5,
  delegation REAL NOT NULL DEFAULT 0.5,
  tool_preference_json TEXT NOT NULL DEFAULT '{}',
  tool_blacklist_json TEXT NOT NULL DEFAULT '[]',
  provider_preference_json TEXT NOT NULL DEFAULT '{}',
  model_preference_json TEXT NOT NULL DEFAULT '{}',
  stats_json TEXT NOT NULL DEFAULT '{}',
  metadata_json TEXT NOT NULL DEFAULT '{}',
  version INTEGER NOT NULL DEFAULT 1,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
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

CREATE INDEX IF NOT EXISTS idx_memory_entities_scope_type
  ON memory_entities(scope_type, scope_id, entity_type, status);
CREATE INDEX IF NOT EXISTS idx_memory_entities_workspace
  ON memory_entities(workspace_id, entity_type, status);
CREATE INDEX IF NOT EXISTS idx_memory_entities_user
  ON memory_entities(user_id, entity_type, status);
CREATE INDEX IF NOT EXISTS idx_memory_relations_source
  ON memory_relations(source_id, status, weight DESC);
CREATE INDEX IF NOT EXISTS idx_memory_relations_target
  ON memory_relations(target_id, status, weight DESC);
CREATE INDEX IF NOT EXISTS idx_memory_relations_workspace
  ON memory_relations(workspace_id, status);
CREATE INDEX IF NOT EXISTS idx_memory_entity_facts_fact
  ON memory_entity_facts(fact_id);
CREATE INDEX IF NOT EXISTS idx_memory_entity_versions_entity
  ON memory_entity_versions(entity_id, version DESC);
CREATE INDEX IF NOT EXISTS idx_agent_evolution_events_agent
  ON agent_evolution_events(agent_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_agent_evolution_events_kind
  ON agent_evolution_events(agent_id, event_kind, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_agent_evolution_proposals_status
  ON agent_evolution_proposals(agent_id, status, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_agent_skill_stats_agent
  ON agent_skill_stats(agent_id, preference_score DESC);
