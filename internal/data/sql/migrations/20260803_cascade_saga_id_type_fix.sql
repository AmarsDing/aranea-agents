-- Version 20260803: Fix cascade_saga_steps id type
-- The original 20260609 migration created id as INTEGER PRIMARY KEY AUTOINCREMENT,
-- but the data layer inserts UUID strings. Since the table was never successfully
-- used (due to a table-name prefix bug fixed in the same patch), it is safe to
-- drop and recreate with TEXT PRIMARY KEY.
DROP TABLE IF EXISTS cascade_saga_steps;
CREATE TABLE cascade_saga_steps (
    id TEXT PRIMARY KEY,
    proposal_id TEXT NOT NULL,
    step_index INTEGER NOT NULL,
    step_name TEXT NOT NULL,
    state TEXT NOT NULL DEFAULT 'pending'
        CHECK(state IN ('pending','running','succeeded','failed','compensated','skipped')),
    is_critical INTEGER NOT NULL DEFAULT 1,
    attempts INTEGER NOT NULL DEFAULT 0,
    started_at TEXT,
    finished_at TEXT,
    payload_json TEXT,
    result_json TEXT,
    error TEXT,
    created_at TEXT NOT NULL DEFAULT '',
    UNIQUE(proposal_id, step_index)
);
CREATE INDEX IF NOT EXISTS idx_cascade_saga_steps_proposal ON cascade_saga_steps(proposal_id, step_index);
