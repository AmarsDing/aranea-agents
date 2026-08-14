-- Tools module batch-3 (C2): bare time-column indexes for tool invocation tables.
-- The composite indexes (tool_key, started_at) / (agent_id, started_at) cannot serve
-- bare time-range scans on Postgres:
--   * tool_invocations 24h summary:  WHERE started_at >= ?
--   * SearchToolInvocations:         ORDER BY started_at DESC with started_at range filters
--   * tool_invocation_audit purge:   WHERE created_at < ? LIMIT <batch> (batch delete)
--   * SearchToolInvocationAudits:    ORDER BY created_at DESC with created_at range filters
CREATE INDEX IF NOT EXISTS idx_tool_invocations_started_at ON tool_invocations(started_at);
CREATE INDEX IF NOT EXISTS idx_tool_invocation_audit_created_at ON tool_invocation_audit(created_at);
