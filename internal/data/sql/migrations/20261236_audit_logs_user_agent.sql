-- 20261236 audit_logs_user_agent: add user_agent column to audit_logs.
-- Same defect class as 20261235 orchestrations_cancel_reason: the ALTER was
-- embedded in sessionMemoryEnsureMonitorSchemaPatches (migration 20260606),
-- which existing databases had already applied and never re-run — leaving the
-- column permanently missing and every InsertAuditLog (monitor.go) failing
-- with pq: column "user_agent" does not exist (42703).
-- Idempotent: IF NOT EXISTS guard makes re-runs safe.

ALTER TABLE audit_logs ADD COLUMN IF NOT EXISTS user_agent TEXT NOT NULL DEFAULT '';
