-- Version 20260725: Bi-temporal validity columns for memory_facts (P3-8).
-- ValidUntil is set when a memory is superseded by a newer conflicting one,
-- instead of deleting the old memory. This preserves history for temporal
-- reconstruction queries. Empty valid_until means the fact is currently valid.
ALTER TABLE memory_facts ADD COLUMN valid_from TEXT NOT NULL DEFAULT '';
ALTER TABLE memory_facts ADD COLUMN valid_until TEXT NOT NULL DEFAULT '';

-- Partial index for filtering currently-valid memories in SearchMemories.
-- SQLite supports partial indexes since 3.8.0 (2014).
CREATE INDEX IF NOT EXISTS idx_memory_facts_valid_until
  ON memory_facts(valid_until)
  WHERE valid_until = '';
