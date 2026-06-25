-- Version 20260902: Drop messages subsystem (Phase 1c-3)
-- The messages table has been superseded by the activities table. All chat
-- history is now projected from runtime events by ActivityProjector
-- (internal/agent/activity_projector.go) and persisted as Activity records.
--
-- Removed components:
--   - internal/data/session_message_repo.go (Ent-backed Message repo)
--   - internal/data/message_search.go (FTS5 search)
--   - internal/data/message_fts_schema.go (FTS5 schema maintainer)
--   - internal/data/ent/schema/message.go (Ent schema)
--   - internal/biz/session/messages.go (MessageReader/Writer interfaces)
--
-- Read path: ActivityMessageReader adapts ActivityLister → MessageReader.
-- Write path: NoopMessageWriter (ActivityProjector handles persistence).
--
-- Idempotent: all statements use IF EXISTS. Backfill uses NOT EXISTS guard.

-- 1. Backfill messages → activities (safety net for pre-AF sessions).
--    Only inserts messages that don't already have a matching Activity
--    (same session_id + turn_id + content). Role is mapped to Activity kind:
--      user → task, assistant → reply, tool → action, system → notice.
INSERT INTO activities (id, kind, status, session_id, turn_id, content, timestamp, agent_key)
SELECT
    'migrated_' || m.id,
    CASE m.role
        WHEN 'user' THEN 'task'
        WHEN 'assistant' THEN 'reply'
        WHEN 'tool' THEN 'action'
        WHEN 'system' THEN 'notice'
        ELSE 'notice'
    END,
    'completed',
    m.session_id,
    m.turn_id,
    m.content_markdown,
    m.created_at,
    ''
FROM messages m
WHERE NOT EXISTS (
    SELECT 1 FROM activities a
    WHERE a.session_id = m.session_id
      AND a.turn_id = m.turn_id
      AND a.content = m.content_markdown
);

-- 2. FTS5 triggers (messages_fts_ai/ad/au) are dropped implicitly when the
--    messages table is dropped in step 5. SQLite and Postgres both cascade
--    trigger drops on DROP TABLE. Postgres never had these triggers anyway.

-- 3. Drop FTS5 virtual table (SQLite) and FTS indexes (Postgres).
--    These statements are idempotent: missing objects are skipped by the
--    migration runner's dialect-aware error handling.
DROP INDEX IF EXISTS idx_messages_tsv;
DROP INDEX IF EXISTS idx_messages_session_id;
DROP TABLE IF EXISTS messages_fts;

-- 4. Drop messages indexes (Ent-managed).
DROP INDEX IF EXISTS idx_messages_session_turn;
DROP INDEX IF EXISTS message_session_id;
DROP INDEX IF EXISTS message_session_id_turn_id;
DROP INDEX IF EXISTS message_session_id_status;

-- 5. Drop messages table (Ent-managed schema).
DROP TABLE IF EXISTS messages;
