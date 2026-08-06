-- 20261128 memory_facts_fts_index (P2-3, Postgres only)
-- GIN index for L3 fact full-text search: tsvector over statement +
-- details_markdown with the 'simple' config. The 'simple' config does not
-- segment CJK runs (continuous Chinese becomes a single token), so FTS is a
-- COMPLEMENTARY signal for alphanumeric tokens (codes, names, IDs) — CJK
-- keyword matching stays with the Go substring channel (keywordOverlapScore).
-- Idempotent: CREATE INDEX IF NOT EXISTS. The registry Func gates this file
-- to Postgres dialect; SQLite CLI/tests skip it.
CREATE INDEX IF NOT EXISTS idx_memory_facts_fts
  ON memory_facts USING GIN (
    to_tsvector('simple', statement || ' ' || COALESCE(details_markdown, ''))
  );
