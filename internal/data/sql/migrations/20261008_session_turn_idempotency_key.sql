-- C-13: session_turns.idempotency_key for canonical turn dedupe.
-- Ent Schema.Create() does not ADD COLUMN on existing tables.

ALTER TABLE session_turns ADD COLUMN idempotency_key TEXT NOT NULL DEFAULT '';

-- Backfill empty keys with id-scoped sentinels so the unique index can be created
-- without colliding on legacy rows that all have ''.
UPDATE session_turns
SET idempotency_key = '__id__:' || id
WHERE idempotency_key = '' OR idempotency_key IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_session_turns_session_idem
  ON session_turns (session_id, idempotency_key);
