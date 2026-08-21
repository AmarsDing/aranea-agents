-- 20261235 orchestrations_cancel_reason: add cancel_reason column to orchestrations.
-- The ALTER was originally embedded in EnsureOrchestrationSchema (20260713) which
-- only runs on fresh table creation; existing PG databases that already applied
-- 20260713 never re-execute it, leaving the column permanently missing.
-- Idempotent: AlreadyExistsErr / IF NOT EXISTS guards make re-runs safe.

ALTER TABLE orchestrations ADD COLUMN IF NOT EXISTS cancel_reason TEXT DEFAULT '';
