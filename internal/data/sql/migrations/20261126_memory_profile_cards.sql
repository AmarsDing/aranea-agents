-- Version 20261126: Profile resident cards (FR-12.7 / report §6.4).
-- One distilled profile card per (agent_id, user_id), maintained by the
-- Sleep-time ProfileCardDistiller from active profile/preference/goal/
-- constraint facts. Injected into the prompt unconditionally (100% inject
-- rate, no recall scoring) at the first memory-block position.
CREATE TABLE IF NOT EXISTS memory_profile_cards (
  id TEXT PRIMARY KEY,
  agent_id TEXT NOT NULL,
  user_id TEXT NOT NULL DEFAULT '',
  content TEXT NOT NULL DEFAULT '',
  fact_count INTEGER NOT NULL DEFAULT 0,
  version INTEGER NOT NULL DEFAULT 1,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_memory_profile_cards_agent_user ON memory_profile_cards(agent_id, user_id);
