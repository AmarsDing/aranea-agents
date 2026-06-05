-- P0: Add missing indexes for high-frequency query paths
-- skill_invocation: session_id index (used by session_repo, session_timeline)
CREATE INDEX IF NOT EXISTS idx_skill_invocation_session ON skill_invocation(session_id);
CREATE INDEX IF NOT EXISTS idx_skill_invocation_agent ON skill_invocation(agent_id);

-- session_turns: run_id, agent_id, team_id indexes
CREATE INDEX IF NOT EXISTS idx_session_turns_run_id ON session_turns(run_id);
CREATE INDEX IF NOT EXISTS idx_session_turns_agent_id ON session_turns(agent_id);
CREATE INDEX IF NOT EXISTS idx_session_turns_team_id ON session_turns(team_id);

-- tool_invocation_audit: session_id index
CREATE INDEX IF NOT EXISTS idx_tool_invocation_audit_session ON tool_invocation_audit(session_id);

-- session_participants: participant_id index
CREATE INDEX IF NOT EXISTS idx_session_participants_participant ON session_participants(participant_id);

-- platform_channel: status+enabled, deleted_at indexes
CREATE INDEX IF NOT EXISTS idx_channel_status_enabled ON channel(status, enabled);
CREATE INDEX IF NOT EXISTS idx_channel_deleted_at ON channel(deleted_at);

-- hooks: status+enabled, deleted_at indexes
CREATE INDEX IF NOT EXISTS idx_hooks_status_enabled ON hooks(status, enabled);
CREATE INDEX IF NOT EXISTS idx_hooks_deleted_at ON hooks(deleted_at);

-- mcp_server: status+enabled, deleted_at indexes
CREATE INDEX IF NOT EXISTS idx_mcp_server_status_enabled ON mcp_server(status, enabled);
CREATE INDEX IF NOT EXISTS idx_mcp_server_deleted_at ON mcp_server(deleted_at);

-- P2: Add deleted_at columns to high-growth tables for soft-delete support
ALTER TABLE tool_invocations ADD COLUMN deleted_at TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_tool_invocations_deleted_at ON tool_invocations(deleted_at);

ALTER TABLE skill_invocation ADD COLUMN deleted_at TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_skill_invocation_deleted_at ON skill_invocation(deleted_at);
