-- Add missing UNIQUE index for memory_episodes ON CONFLICT(session_id, title, agent_id) upsert
CREATE UNIQUE INDEX IF NOT EXISTS idx_memory_episodes_session_title_agent ON memory_episodes(session_id, title, agent_id);
