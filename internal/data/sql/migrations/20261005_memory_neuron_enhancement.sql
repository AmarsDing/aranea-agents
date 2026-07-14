-- Version 20261005: Neuron model field enhancement for memory_entities/memory_relations.
-- Supports FR-10.1 (activation spreading) and FR-10.2 (Hebbian co-activation counting).
-- Note: 20260728 was already used by memory_job_deadletter_schema, so this migration
-- uses 20261005 (the next available version after 20261004 memory_context_note).
--
-- memory_entities new columns:
--   activation           — current spreading-activation value (0.0~1.0, decays over time)
--   activation_updated_at — RFC3339 timestamp of last activation update (for decay computation)
--   source_type           — perception/inference/told/knowledge (distinguishes how the
--                           entity was acquired, separate from existing source_kind='extracted')
--   valence               — reserved: emotional valence (positive/negative affect)
--   arousal               — reserved: emotional arousal (activation level of affect)
--
-- memory_relations new columns:
--   co_activation_count  — Hebbian counter: how often both endpoints fired together
--   last_reinforced_at   — RFC3339 timestamp of last Hebbian reinforcement
--   context_note         — A-MEM style contextual annotation explaining relationship evolution

ALTER TABLE memory_entities ADD COLUMN activation REAL NOT NULL DEFAULT 0;
ALTER TABLE memory_entities ADD COLUMN activation_updated_at TEXT NOT NULL DEFAULT '';
ALTER TABLE memory_entities ADD COLUMN source_type TEXT NOT NULL DEFAULT '';
ALTER TABLE memory_entities ADD COLUMN valence REAL NOT NULL DEFAULT 0;
ALTER TABLE memory_entities ADD COLUMN arousal REAL NOT NULL DEFAULT 0;

ALTER TABLE memory_relations ADD COLUMN co_activation_count INTEGER NOT NULL DEFAULT 0;
ALTER TABLE memory_relations ADD COLUMN last_reinforced_at TEXT NOT NULL DEFAULT '';
ALTER TABLE memory_relations ADD COLUMN context_note TEXT NOT NULL DEFAULT '';

-- Spreading activation index: Top-K retrieval by activation descending.
-- Covers the common query "find the most active neurons in scope S".
CREATE INDEX IF NOT EXISTS idx_memory_entities_activation
  ON memory_entities(scope_type, scope_id, status, activation DESC);

-- Graph traversal index: accelerates recursive CTE join on (source_id, target_id)
-- filtered by status and ordered by weight for Top-K edge selection.
CREATE INDEX IF NOT EXISTS idx_memory_relations_graph
  ON memory_relations(source_id, target_id, status, weight DESC);
