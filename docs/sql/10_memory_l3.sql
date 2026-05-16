-- ============================================================
-- Memory L3: 语义记忆
-- 表: memory_facts, memory_fact_versions, memory_fact_feedback,
--     memory_fact_conflicts, memory_fact_index
-- ============================================================

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
