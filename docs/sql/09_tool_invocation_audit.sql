-- Tool invocation audit trail (retention managed by ops; default policy 90 days).
CREATE TABLE IF NOT EXISTS tool_invocation_audit (
  id TEXT PRIMARY KEY,
  invocation_id TEXT NOT NULL DEFAULT '',
  tool_key TEXT NOT NULL,
  agent_id TEXT NOT NULL DEFAULT '',
  user_id TEXT NOT NULL DEFAULT '',
  session_id TEXT NOT NULL DEFAULT '',
  action TEXT NOT NULL DEFAULT 'tool.call',
  result_summary TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'success',
  source TEXT NOT NULL DEFAULT 'adk',
  created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_tool_invocation_audit_tool_time ON tool_invocation_audit(tool_key, created_at);
CREATE INDEX IF NOT EXISTS idx_tool_invocation_audit_agent_time ON tool_invocation_audit(agent_id, created_at);
CREATE INDEX IF NOT EXISTS idx_tool_invocation_audit_user_time ON tool_invocation_audit(user_id, created_at);
