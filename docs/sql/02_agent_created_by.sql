-- Migration: add agents.created_by for list creator filter (LIST-02).
ALTER TABLE agents ADD COLUMN created_by TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_agents_created_by ON agents (created_by) WHERE deleted_at = '';
