-- 20261242 memory_facts_trgm_index (P1-2, Postgres only)
-- CJK / short-query channel for L3 fact recall. Postgres 'simple' tsvector
-- does not segment continuous Chinese, so FTS rarely matches 喜欢蓝色-style
-- queries. word_similarity + gin_trgm_ops is the same operator knowledge
-- and agent-case already use (%> = query trigrams vs a contiguous span).
-- Idempotent: CREATE EXTENSION / CREATE INDEX IF NOT EXISTS. The registry
-- Func gates this file to Postgres; SQLite CLI/tests skip it.
CREATE EXTENSION IF NOT EXISTS pg_trgm;
CREATE INDEX IF NOT EXISTS idx_memory_facts_statement_trgm
  ON memory_facts USING GIN (statement gin_trgm_ops);
