-- Version 20260826: event_dead_letter table for the v2 sequencer dead-letter
-- store (P1-R2b). Persists events whose entity upsert failed permanently so
-- they can be replayed after restart, following the memory_job_deadletter
-- pattern (20260728). The event_dead_letter_repo.go constructor retains a
-- safety-net ensureTable() for the Wire-construction-vs-P1-migration race,
-- but this migration is the primary, versioned creation path.

CREATE TABLE IF NOT EXISTS event_dead_letter (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    event_kind   TEXT    NOT NULL,
    entity_kind  TEXT    NOT NULL,
    entity_op    TEXT    NOT NULL DEFAULT 'upsert',
    entity_id    TEXT    NOT NULL DEFAULT '',
    session_id   TEXT    NOT NULL DEFAULT '',
    payload_json TEXT    NOT NULL DEFAULT '{}',
    attempts     INTEGER NOT NULL DEFAULT 0,
    last_error   TEXT    NOT NULL DEFAULT '',
    state        TEXT    NOT NULL DEFAULT 'pending'
                 CHECK(state IN ('pending','replayed','abandoned')),
    failed_at    INTEGER NOT NULL,
    updated_at   INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_event_dl_state ON event_dead_letter(state, failed_at);
-- Atomic upsert dedup: one pending row per entity (mirrors the in-memory
-- ring's entity-ID dedup semantics).
CREATE UNIQUE INDEX IF NOT EXISTS idx_event_dl_unique_pending ON event_dead_letter(entity_kind, entity_id) WHERE state = 'pending';
