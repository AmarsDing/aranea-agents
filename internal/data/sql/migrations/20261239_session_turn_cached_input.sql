-- 20261239 session_turn_cached_input: add cached_input_tokens column to session_turns.
-- Turn-level prompt-cache hit count (provider-reported, cache-hit portion of
-- input_tokens). Ent Schema.Create() does not ALTER pre-existing tables, so
-- databases created before this column existed need this explicit patch.
-- Idempotent: IF NOT EXISTS guard makes re-runs safe.

ALTER TABLE session_turns ADD COLUMN IF NOT EXISTS cached_input_tokens INTEGER NOT NULL DEFAULT 0;
