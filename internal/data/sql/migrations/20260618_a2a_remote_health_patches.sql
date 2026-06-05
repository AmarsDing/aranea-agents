-- Version 20260618: A2A remote agent health column patches
ALTER TABLE a2a_remote_agents ADD COLUMN last_health_at TEXT NOT NULL DEFAULT '';
ALTER TABLE a2a_remote_agents ADD COLUMN last_health_ok INTEGER NOT NULL DEFAULT 0;
ALTER TABLE a2a_remote_agents ADD COLUMN last_health_error TEXT NOT NULL DEFAULT '';
