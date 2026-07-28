-- Version 20261113: A2A remote agents org_id column for federation
ALTER TABLE a2a_remote_agents ADD COLUMN org_id TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_a2a_remote_agents_org_id ON a2a_remote_agents(org_id);
