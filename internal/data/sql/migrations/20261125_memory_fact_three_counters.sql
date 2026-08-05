-- Version 20261125: L3 fact three-stage counters (FR-12.6 / report §6.5).
-- Replaces the semantically-wrong use_count ("recalled but counted as used")
-- with three precise counters:
--   recalled_count  — entered a recall result set (backfilled from use_count)
--   injected_count  — actually written into the LLM prompt (the only "usage"
--                     count worth showing to users)
--   cited_count     — explicitly referenced by the assistant reply
-- use_count is retained but no longer maintained (historical data preserved).
ALTER TABLE memory_facts ADD COLUMN recalled_count INTEGER NOT NULL DEFAULT 0;
ALTER TABLE memory_facts ADD COLUMN injected_count INTEGER NOT NULL DEFAULT 0;
ALTER TABLE memory_facts ADD COLUMN cited_count INTEGER NOT NULL DEFAULT 0;

-- One-time backfill: historical use_count was incremented by the recall path,
-- so its semantics map to recalled_count. The WHERE guard keeps the statement
-- idempotent on re-run (fresh columns are 0, already-backfilled rows skip).
-- NOTE: no semicolons inside comments — splitDDLStatements is comment-unaware.
UPDATE memory_facts SET recalled_count = use_count WHERE recalled_count = 0 AND use_count > 0;

-- Citation dedup ledger: one row per (fact, turn) citation so the backfill
-- worker can re-scan overlapping windows without double-counting.
CREATE TABLE IF NOT EXISTS memory_fact_citations (
  fact_id TEXT NOT NULL,
  turn_id TEXT NOT NULL,
  created_at TEXT NOT NULL,
  PRIMARY KEY (fact_id, turn_id)
);
CREATE INDEX IF NOT EXISTS idx_memory_fact_citations_turn ON memory_fact_citations(turn_id);
