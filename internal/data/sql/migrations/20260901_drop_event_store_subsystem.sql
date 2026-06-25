-- Version 20260901: Drop event_store subsystem (Phase 1c-2)
-- The event_store subsystem (event_store + event_wal tables) has been removed.
-- Activity records (activities table) now serve as the durable source of truth
-- for session event history, accessed via the ListActivities RPC.
--
-- Removed components:
--   - internal/event/wal.go (EventWAL — Write-Before-Publish-Fanout)
--   - internal/event/postgres_eventstore.go (PostgresEventStore — cross-process replay)
--   - internal/data/event_store_repo.go (Ent-backed EventStore repo)
--   - internal/biz/event_persist_handler.go (envelope persistence)
--   - internal/service/event.go (EventService gRPC/HTTP API)
--   - api/kratos/event/v1/*.proto + generated *.pb.go
--
-- WS reconnect replay now uses the in-memory event.Buffer for live events
-- and ListActivities RPC for full session history.
--
-- Idempotent: all statements use IF EXISTS.

-- 1. Drop event_store foreign key constraint (if exists)
DO $$ BEGIN
    IF EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'fk_event_store_session'
    ) THEN
        ALTER TABLE event_store
            DROP CONSTRAINT fk_event_store_session;
    END IF;
END $$;

-- 2. Drop event_store indexes
DROP INDEX IF EXISTS idx_event_store_session_created;
DROP INDEX IF EXISTS idx_event_store_session_type;

-- 3. Drop event_store table (Ent-managed schema)
DROP TABLE IF EXISTS event_store;

-- 4. Drop event_wal indexes
DROP INDEX IF EXISTS idx_event_wal_unpublished;

-- 5. Drop event_wal table (Postgres-native schema from postgres_wal_storage.go)
DROP TABLE IF EXISTS event_wal;
