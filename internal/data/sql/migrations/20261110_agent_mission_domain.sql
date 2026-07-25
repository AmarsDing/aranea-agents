-- Version 20261110: Agents — mission_statement / domain_path columns (B.10.21 使命驱动匹配)
-- Idempotent: "duplicate column" errors are treated as success by the migration runner (DB-N6).
ALTER TABLE agents ADD COLUMN mission_statement TEXT NOT NULL DEFAULT '';
ALTER TABLE agents ADD COLUMN domain_path VARCHAR(256) NOT NULL DEFAULT '';
