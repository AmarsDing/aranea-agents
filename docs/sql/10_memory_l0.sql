-- ============================================================
-- Memory L0: 上下文组装与摘要
-- 表: memory_l0_assembly_snapshots, memory_items, session_summaries
-- ============================================================

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
