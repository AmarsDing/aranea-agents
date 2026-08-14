-- Version 20261215: knowledge chunk cited_count + citation dedup ledger (29-token P2-2).
-- Knowledge-side counterpart of 20261125_memory_fact_three_counters (cited stage only):
-- knowledge chunks have no recalled/injected counters (recall path is tool-mediated,
-- not prompt-injected), so only the cited stage is tracked.
--   cited_count — explicitly referenced by the assistant reply (citation backfill
--                 worker, heuristic + dedup ledger)
-- Idempotent: IF NOT EXISTS guards; safe to re-apply.

ALTER TABLE knowledge_chunks ADD COLUMN IF NOT EXISTS cited_count INT NOT NULL DEFAULT 0;

-- Citation dedup ledger: one row per (chunk, turn) citation so the backfill
-- worker can re-scan overlapping windows without double-counting.
-- NOTE: no semicolons inside comments — splitDDLStatements is comment-unaware.
CREATE TABLE IF NOT EXISTS knowledge_chunk_citations (
  chunk_id TEXT NOT NULL,
  turn_id TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  PRIMARY KEY (chunk_id, turn_id)
);
CREATE INDEX IF NOT EXISTS idx_knowledge_chunk_citations_turn ON knowledge_chunk_citations(turn_id);
