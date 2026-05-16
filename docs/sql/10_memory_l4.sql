-- ============================================================
-- Memory L4: 持久化/实体关系记忆
-- 表: memory_entities, memory_relations, memory_entity_facts,
--     memory_entity_versions, agent_identity, agent_strategy_profile
-- ============================================================

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
