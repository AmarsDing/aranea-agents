-- session_metrics: 高频更新的 metrics 字段拆分表
CREATE TABLE IF NOT EXISTS session_metrics (
    session_id TEXT PRIMARY KEY,
    message_count INTEGER DEFAULT 0,
    run_count INTEGER DEFAULT 0,
    model_call_count INTEGER DEFAULT 0,
    tool_call_count INTEGER DEFAULT 0,
    skill_call_count INTEGER DEFAULT 0,
    mcp_call_count INTEGER DEFAULT 0,
    input_tokens INTEGER DEFAULT 0,
    output_tokens INTEGER DEFAULT 0,
    total_tokens INTEGER DEFAULT 0,
    total_cost_micro_usd INTEGER DEFAULT 0,
    avg_latency_ms REAL DEFAULT 0,
    error_count INTEGER DEFAULT 0,
    context_used_tokens INTEGER DEFAULT 0,
    context_used_ratio REAL DEFAULT 0,
    max_context_used_ratio REAL DEFAULT 0,
    context_status TEXT DEFAULT '',
    last_message_at TEXT DEFAULT '',
    updated_at TEXT DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);

-- session_runtime: 运行时快照字段拆分表
CREATE TABLE IF NOT EXISTS session_runtime (
    session_id TEXT PRIMARY KEY,
    session_revision INTEGER DEFAULT 0,
    state_json TEXT DEFAULT '{}',
    runner_snapshot_json TEXT DEFAULT '',
    metadata_json TEXT DEFAULT '{}',
    compress_version INTEGER DEFAULT 0,
    updated_at TEXT DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);

-- 回填 session_metrics
INSERT OR IGNORE INTO session_metrics (session_id, message_count, run_count, model_call_count, tool_call_count, skill_call_count, mcp_call_count, input_tokens, output_tokens, total_tokens, total_cost_micro_usd, avg_latency_ms, error_count, context_used_tokens, context_used_ratio, max_context_used_ratio, context_status, last_message_at, updated_at)
SELECT id, message_count, run_count, model_call_count, tool_call_count, skill_call_count, mcp_call_count, input_tokens, output_tokens, total_tokens, total_cost_micro_usd, avg_latency_ms, error_count, context_used_tokens, context_used_ratio, max_context_used_ratio, context_status, last_message_at, updated_at FROM sessions WHERE deleted_at = '' OR deleted_at IS NULL;

-- 回填 session_runtime
INSERT OR IGNORE INTO session_runtime (session_id, session_revision, state_json, runner_snapshot_json, metadata_json, compress_version, updated_at)
SELECT id, session_revision, COALESCE(state_json, '{}'), COALESCE(runner_snapshot_json, ''), COALESCE(metadata_json, '{}'), compress_version, updated_at FROM sessions WHERE deleted_at = '' OR deleted_at IS NULL;
