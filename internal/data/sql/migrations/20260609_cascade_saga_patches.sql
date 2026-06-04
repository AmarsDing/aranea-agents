-- Version 20260609: Cascade saga patches
CREATE TABLE IF NOT EXISTS cascade_saga_steps (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
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
    UNIQUE(proposal_id, step_index)
);
CREATE INDEX IF NOT EXISTS idx_cascade_saga_steps_proposal ON cascade_saga_steps(proposal_id, step_index);
ALTER TABLE memory_facts ADD COLUMN last_cascade_original_statement TEXT NOT NULL DEFAULT '';
