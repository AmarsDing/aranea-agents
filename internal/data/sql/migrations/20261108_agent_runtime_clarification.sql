-- Version 20261108: Agent runtime settings — clarification gate column (P-CLARIFY B.10.18)
-- Idempotent: "duplicate column" errors are treated as success by the migration runner (DB-N6).
ALTER TABLE agent_runtime_settings ADD COLUMN clarification_enabled INTEGER NOT NULL DEFAULT 1;
