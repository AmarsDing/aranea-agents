-- Version 20260617: Postgres Phase 1 migration
-- Migrates WAL/EventStore/Checkpoint critical tables to Postgres-native schema.
-- This file is executed ONLY on the Postgres connection (d.Postgres()),
-- NOT on the SQLite rawDB. SQLite retains its own event_wal schema
-- (see internal/event/wal_storage.go ensureSchema).
--
-- Postgres-specific differences from SQLite schema:
--   - TIMESTAMPTZ instead of DATETIME/TEXT (timezone-aware timestamps)
--   - $N placeholders instead of ? (handled in Go code)
--   - ON CONFLICT (id) DO NOTHING instead of INSERT OR IGNORE
--   - BIGINT/INTEGER strict typing
--   - Native partial unique indexes (no special syntax needed)
--
-- Idempotent: all statements use IF NOT EXISTS / ON CONFLICT DO NOTHING.

-- 1. event_wal: Critical event Write-Ahead Log (WBPF)
--    Mirrors internal/event/postgres_wal_storage.go ensureSchema.
CREATE TABLE IF NOT EXISTS event_wal (
    id TEXT PRIMARY KEY,
    envelope_json TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    published_at TIMESTAMPTZ,
    published INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_event_wal_unpublished
    ON event_wal(published, created_at);

-- 2. event_store: persisted Envelope snapshots for WS replay
--    Mirrors internal/data/ent/schema/eventstore.go schema.
CREATE TABLE IF NOT EXISTS event_store (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL,
    type TEXT NOT NULL,
    author TEXT NOT NULL DEFAULT '',
    channel TEXT NOT NULL DEFAULT '',
    envelope_json TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_event_store_session_created
    ON event_store(session_id, created_at);
CREATE INDEX IF NOT EXISTS idx_event_store_session_type
    ON event_store(session_id, type);

-- 3. session_run_checkpoints: durable resume snapshots (CC-R-03 / CC-F-02)
--    Mirrors internal/data/ent/schema/session_run_checkpoint.go schema.
CREATE TABLE IF NOT EXISTS session_run_checkpoints (
    id TEXT PRIMARY KEY,
    session_run_id TEXT NOT NULL,
    session_id TEXT NOT NULL,
    turn_id TEXT NOT NULL DEFAULT '',
    agent_id TEXT NOT NULL DEFAULT '',
    payload_json TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_session_run_checkpoints_run
    ON session_run_checkpoints(session_run_id);

-- 4. Invariant constraints (Postgres native partial unique indexes)
--    INV-UNIQ-01: One Session has at most one active Run.
CREATE UNIQUE INDEX IF NOT EXISTS idx_session_runs_active_unique
    ON session_runs(session_id)
    WHERE phase NOT IN ('completed', 'failed', 'cancelled');

--    INV-UNIQ-02: One Team has at most one active Run.
CREATE UNIQUE INDEX IF NOT EXISTS idx_team_runs_active_unique
    ON team_runs(team_id)
    WHERE status NOT IN ('success', 'failed', 'cancelled');

-- 5. Foreign Key constraints (Postgres supports ALTER TABLE ADD CONSTRAINT)
--    INV-REF-01: session_run_checkpoints.session_run_id → session_runs.id
--    Added here because Postgres supports native FK; SQLite defers to app-layer
--    (see 20260724_invariant_constraints.sql TECH-DEBT note).
--    Wrapped in DO $$ blocks for idempotency (check existence before adding).
DO $$ BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'fk_session_run_checkpoints_run'
    ) THEN
        ALTER TABLE session_run_checkpoints
            ADD CONSTRAINT fk_session_run_checkpoints_run
            FOREIGN KEY (session_run_id) REFERENCES session_runs(id) ON DELETE CASCADE;
    END IF;
END $$;

--    INV-REF-02: event_store.session_id → sessions.id
DO $$ BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'fk_event_store_session'
    ) THEN
        ALTER TABLE event_store
            ADD CONSTRAINT fk_event_store_session
            FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE;
    END IF;
END $$;

--    INV-REF-03: session_run_checkpoints.session_id → sessions.id
DO $$ BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'fk_session_run_checkpoints_session'
    ) THEN
        ALTER TABLE session_run_checkpoints
            ADD CONSTRAINT fk_session_run_checkpoints_session
            FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE;
    END IF;
END $$;
