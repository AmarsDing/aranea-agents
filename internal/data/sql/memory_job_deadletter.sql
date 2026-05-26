-- MEM-OPT-03: Dead-letter store for AutoMemoryQueue dropped jobs.
-- Applied lazily via ensureMemoryJobDeadletterTable().

CREATE TABLE IF NOT EXISTS memory_job_deadletter (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    enqueued_at      INTEGER NOT NULL,               -- unix ms
    failed_at        INTEGER NOT NULL,               -- unix ms
    session_id       TEXT    NOT NULL DEFAULT '',
    app_name         TEXT    NOT NULL DEFAULT '',
    user_id          TEXT    NOT NULL DEFAULT '',
    feedback_msg_id  TEXT    NOT NULL DEFAULT '',
    payload_json     TEXT    NOT NULL DEFAULT '{}',
    drop_reason      TEXT    NOT NULL,               -- queue_full | quota_exceeded | extract_failed_terminal
    priority         INTEGER NOT NULL DEFAULT 1,     -- 0=high 1=normal 2=low
    attempts         INTEGER NOT NULL DEFAULT 0,
    last_error       TEXT    NOT NULL DEFAULT '',
    state            TEXT    NOT NULL DEFAULT 'pending'
                     CHECK(state IN ('pending','replayed','abandoned'))
);

CREATE INDEX IF NOT EXISTS idx_memory_job_dl_state_enq ON memory_job_deadletter(state, enqueued_at);
CREATE INDEX IF NOT EXISTS idx_memory_job_dl_session   ON memory_job_deadletter(session_id);
