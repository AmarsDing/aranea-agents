-- Version 20261230: Agent runtime settings — reply reminder gate column
-- Idempotent: "duplicate column" errors are treated as success by the migration runner (DB-N6).
ALTER TABLE agent_runtime_settings ADD COLUMN reply_reminder_enabled INTEGER NOT NULL DEFAULT 1;
