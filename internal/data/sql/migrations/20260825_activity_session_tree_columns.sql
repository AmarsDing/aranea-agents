-- Phase 1a/2: Add tool_category and stage columns to activities table.
-- tool_category classifies tools by functional type (shell/browser/file_read/...)
-- for frontend rendering. stage tracks the current phase for session/team_stage/
-- graph_stage kind activities.

ALTER TABLE activities ADD COLUMN tool_category TEXT NOT NULL DEFAULT '';
ALTER TABLE activities ADD COLUMN stage TEXT NOT NULL DEFAULT '';

-- Phase 2: Add session tree hierarchy columns to sessions table.
-- session_type classifies the session's role (spirit/team/agent/standalone).
-- member_agent_key identifies the executing agent for agent-type sessions.
-- member_role is the agent's role within a team (coordinator/worker).
-- execution_stage/completed_steps/total_steps/progress_pct track execution progress.

ALTER TABLE sessions ADD COLUMN session_type TEXT NOT NULL DEFAULT 'standalone';
ALTER TABLE sessions ADD COLUMN member_agent_key TEXT NOT NULL DEFAULT '';
ALTER TABLE sessions ADD COLUMN member_role TEXT NOT NULL DEFAULT '';
ALTER TABLE sessions ADD COLUMN execution_stage TEXT NOT NULL DEFAULT '';
ALTER TABLE sessions ADD COLUMN completed_steps INTEGER NOT NULL DEFAULT 0;
ALTER TABLE sessions ADD COLUMN total_steps INTEGER NOT NULL DEFAULT 0;
ALTER TABLE sessions ADD COLUMN progress_pct REAL NOT NULL DEFAULT 0.0;

-- Backfill session_type for existing sessions.
-- Root sessions (no parent) become 'standalone' (default); spirit sessions are
-- identified by having a root_session_id equal to their own id and no parent.
UPDATE sessions SET session_type = 'spirit'
WHERE parent_session_id = '' AND root_session_id = id AND id != '';
-- Team sessions have a parent and a team_id but no member_agent_key.
UPDATE sessions SET session_type = 'team'
WHERE parent_session_id != '' AND team_id != '' AND member_agent_key = '';
-- Agent sessions (formerly "member") have a parent and member_agent_key.
UPDATE sessions SET session_type = 'agent'
WHERE parent_session_id != '' AND member_agent_key != '';

-- Backfill activity.tool_category based on tool_name prefix/name matching.
UPDATE activities SET tool_category = 'shell'
WHERE tool_category = '' AND (tool_name LIKE 'shell%' OR tool_name LIKE 'bash%');
UPDATE activities SET tool_category = 'browser'
WHERE tool_category = '' AND (tool_name LIKE 'browser%' OR tool_name LIKE 'playwright%');
UPDATE activities SET tool_category = 'file_read'
WHERE tool_category = '' AND tool_name IN ('read_file', 'cat', 'head');
UPDATE activities SET tool_category = 'file_write'
WHERE tool_category = '' AND tool_name IN ('write_file', 'edit_file', 'patch');
UPDATE activities SET tool_category = 'file_search'
WHERE tool_category = '' AND tool_name IN ('find', 'grep', 'glob');
UPDATE activities SET tool_category = 'web_search'
WHERE tool_category = '' AND tool_name IN ('web_search', 'search');
UPDATE activities SET tool_category = 'todo'
WHERE tool_category = '' AND tool_name IN ('todo_write', 'todo_read');
UPDATE activities SET tool_category = 'mcp'
WHERE tool_category = '' AND tool_name LIKE 'mcp_%';
UPDATE activities SET tool_category = 'code'
WHERE tool_category = '' AND tool_name IN ('execute_code', 'python');

-- Fix spirit_session_id: when empty or equal to session_id, derive from
-- the session's root_session_id.
UPDATE activities
SET spirit_session_id = (
    SELECT s.root_session_id FROM sessions s WHERE s.id = activities.session_id
)
WHERE (spirit_session_id = '' OR spirit_session_id = session_id)
  AND EXISTS (SELECT 1 FROM sessions s WHERE s.id = activities.session_id AND s.root_session_id != '');
