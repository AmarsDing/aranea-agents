-- Version 20260801: Add unique partial index on memory_job_deadletter to prevent
-- duplicate pending rows (P0-5 fix for WriteMemoryDeadLetter race condition).
--
-- The original WriteMemoryDeadLetter used UPDATE-then-INSERT which could race:
-- two concurrent goroutines could both see 0 rows updated and both INSERT,
-- producing duplicate pending rows for the same (session_id, app_name, priority).
--
-- This migration:
-- 1. Deduplicates existing pending rows (keeps the one with highest attempts).
-- 2. Creates a unique partial index so INSERT ON CONFLICT can be used for atomic upsert.
--
-- The partial index only covers state='pending' rows, allowing multiple
-- replayed/abandoned rows for the same (session_id, app_name, priority).

-- Step 1: Deduplicate existing pending rows.
-- For each (session_id, app_name, priority) group with state='pending',
-- keep the row with the highest attempts (most retried), delete the rest.
DELETE FROM memory_job_deadletter
WHERE state = 'pending'
  AND id NOT IN (
    SELECT id FROM (
      SELECT id, ROW_NUMBER() OVER (
        PARTITION BY session_id, app_name, priority
        ORDER BY attempts DESC, failed_at DESC, id DESC
      ) AS rn
      FROM memory_job_deadletter
      WHERE state = 'pending'
    ) WHERE rn = 1
  );

-- Step 2: Create unique partial index.
-- SQLite supports partial indexes since 3.8.0. Postgres since 7.2.
-- IF NOT EXISTS makes this idempotent (DB-N6).
CREATE UNIQUE INDEX IF NOT EXISTS idx_memory_job_dl_unique_pending
ON memory_job_deadletter(session_id, app_name, priority)
WHERE state = 'pending';
