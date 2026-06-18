-- Postgres tsvector full-text search schema for messages.
-- Idempotent: uses IF NOT EXISTS / DO $$ blocks.
-- The tsv column is a GENERATED STORED tsvector that indexes content_markdown
-- using the 'simple' configuration (language-agnostic; works for CJK + English).
-- A GIN index enables fast full-text search.

-- Add tsv column if not exists.
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_schema = 'public' AND table_name = 'messages' AND column_name = 'tsv'
  ) THEN
    ALTER TABLE messages ADD COLUMN tsv tsvector
      GENERATED ALWAYS AS (to_tsvector('simple', coalesce(content_markdown, ''))) STORED;
  END IF;
END $$;

-- Backfill is automatic for GENERATED STORED columns: Postgres computes the
-- value for all existing rows at ALTER TABLE time. No explicit backfill needed.

-- GIN index for fast tsvector search.
CREATE INDEX IF NOT EXISTS idx_messages_tsv ON messages USING GIN (tsv);

-- Index on session_id for filtering (often combined with FTS).
CREATE INDEX IF NOT EXISTS idx_messages_session_id ON messages(session_id);
