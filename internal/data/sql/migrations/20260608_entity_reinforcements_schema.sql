-- Version 20260608: Entity reinforcements schema
CREATE TABLE IF NOT EXISTS entity_reinforcements (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  entity_id TEXT NOT NULL,
  signal TEXT NOT NULL CHECK(signal IN ('hit','confirmed','refuted','edited')),
  occurred_at INTEGER NOT NULL,
  source TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_entity_reinforcements_entity_time
  ON entity_reinforcements(entity_id, occurred_at DESC);
