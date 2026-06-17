-- P0: Add DB-level invariant constraints (INV-UNIQ-01/02, INV-REF-01/02/03)
-- Prevents "one Session/Team multiple active Runs" via partial unique indexes.
-- SQLite supports partial unique indexes natively.

-- INV-UNIQ-01: One Session has at most one active Run (phase NOT terminal).
-- Terminal phases: completed, failed, cancelled.
-- Active phases: interactive, durable.
CREATE UNIQUE INDEX IF NOT EXISTS idx_session_runs_active_unique
    ON session_runs(session_id)
    WHERE phase NOT IN ('completed', 'failed', 'cancelled');

-- INV-UNIQ-02: One Team has at most one active Run (status NOT terminal).
-- Terminal statuses: success, failed, cancelled.
-- Active statuses: pending, running, waiting_human.
CREATE UNIQUE INDEX IF NOT EXISTS idx_team_runs_active_unique
    ON team_runs(team_id)
    WHERE status NOT IN ('success', 'failed', 'cancelled');

-- Note on FK constraints (INV-REF-01/02/03):
-- SQLite does not support ALTER TABLE ADD FOREIGN KEY on existing tables.
-- Adding FK requires table rebuild (CREATE TABLE _new + INSERT + DROP + RENAME),
-- which is high-risk on production data. FK enforcement is deferred to:
--   1. Application-layer guards (already present in Usecase)
--   2. Future table-rebuild migration when schema stabilizes
-- The partial unique indexes above are the highest-value invariant constraints
-- and are safe to add without table rebuild.
-- TECH-DEBT(INV-REF): FK constraints deferred — track in follow-up issue for
-- table-rebuild migration when schema stabilizes.
