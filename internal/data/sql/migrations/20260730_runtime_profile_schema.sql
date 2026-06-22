-- runtime_profile: per-agent runtime configuration profiles
-- Stores tool/skill/knowledge/workspace/prompt policies that override
-- agent defaults at run time.
CREATE TABLE IF NOT EXISTS runtime_profiles (
  id TEXT PRIMARY KEY,
  agent_id TEXT NOT NULL DEFAULT '',
  name TEXT NOT NULL DEFAULT '',
  description TEXT NOT NULL DEFAULT '',
  version TEXT NOT NULL DEFAULT '1',
  is_active INTEGER NOT NULL DEFAULT 0,
  priority INTEGER NOT NULL DEFAULT 0,
  -- JSON-encoded policy blobs (see biz.RuntimeProfile)
  prompt_config TEXT NOT NULL DEFAULT '{}',
  tool_policy TEXT NOT NULL DEFAULT '{}',
  skill_policy TEXT NOT NULL DEFAULT '{}',
  knowledge_policy TEXT NOT NULL DEFAULT '{}',
  workspace_policy TEXT NOT NULL DEFAULT '{}',
  credential_policy TEXT NOT NULL DEFAULT '{}',
  isolation_policy TEXT NOT NULL DEFAULT '{}',
  extra_model_config TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_rp_agent_active ON runtime_profiles(agent_id, is_active);
CREATE INDEX IF NOT EXISTS idx_rp_priority ON runtime_profiles(priority DESC);
