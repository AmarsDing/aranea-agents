-- Version 20260728: memory_job_deadletter table for persistent dead-letter store.
-- Replaces the lazy ensureTable() in memory_job_deadletter.go (DB-R4 compliance).
-- Stores AutoMemory jobs that couldn't be enqueued (queue_full / quota_exceeded)
-- and background job failures (retry_exhausted via DeadLetterSinkAdapter).
-- The memory_job_deadletter.go constructor retains a safety-net ensureTable()
-- for the Wire-construction-vs-P1-migration race, but this migration is the
-- primary, versioned creation path.

CREATE TABLE IF NOT EXISTS memory_job_deadletter (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    enqueued_at      INTEGER NOT NULL,
    failed_at        INTEGER NOT NULL,
    session_id       TEXT    NOT NULL DEFAULT '',
    app_name         TEXT    NOT NULL DEFAULT '',
    user_id          TEXT    NOT NULL DEFAULT '',
    feedback_msg_id  TEXT    NOT NULL DEFAULT '',
    payload_json     TEXT    NOT NULL DEFAULT '{}',
    drop_reason      TEXT    NOT NULL,
    priority         INTEGER NOT NULL DEFAULT 1,
    attempts         INTEGER NOT NULL DEFAULT 0,
    last_error       TEXT    NOT NULL DEFAULT '',
    state            TEXT    NOT NULL DEFAULT 'pending'
                     CHECK(state IN ('pending','replayed','abandoned'))
);

CREATE INDEX IF NOT EXISTS idx_memory_job_dl_state_enq ON memory_job_deadletter(state, enqueued_at);
CREATE INDEX IF NOT EXISTS idx_memory_job_dl_session   ON memory_job_deadletter(session_id);
