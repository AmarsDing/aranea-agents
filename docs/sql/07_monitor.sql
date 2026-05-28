-- ============================================================
-- Monitor 相关表: monitor_events, monitor_traces, monitor_trace_spans, audit_logs
-- ============================================================

CREATE TABLE IF NOT EXISTS audit_logs (
  id TEXT PRIMARY KEY,
  action TEXT NOT NULL,
  resource TEXT NOT NULL,
  resource_id TEXT NOT NULL,
  request_id TEXT NOT NULL,
  detail TEXT NOT NULL,
  created_at TEXT NOT NULL,
  actor TEXT NOT NULL DEFAULT '',
  ip TEXT NOT NULL DEFAULT '',
  user_agent TEXT NOT NULL DEFAULT '',
  severity TEXT NOT NULL DEFAULT '',
  metadata_json TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_audit_logs_action ON audit_logs(action);
CREATE INDEX IF NOT EXISTS idx_audit_logs_resource ON audit_logs(resource);
CREATE INDEX IF NOT EXISTS idx_audit_logs_created_at ON audit_logs(created_at);

CREATE TABLE IF NOT EXISTS monitor_events (
  id TEXT PRIMARY KEY,
  event_key TEXT NOT NULL DEFAULT '',
  name TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'ok',
  metadata_json TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL DEFAULT '',
  deleted_at TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_monitor_events_event_key ON monitor_events(event_key);
CREATE INDEX IF NOT EXISTS idx_monitor_events_status ON monitor_events(status);
CREATE INDEX IF NOT EXISTS idx_monitor_events_created_at ON monitor_events(created_at);

CREATE TABLE IF NOT EXISTS monitor_traces (
  id TEXT PRIMARY KEY,
  trace_key TEXT NOT NULL DEFAULT '',
  name TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'running',
  metadata_json TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL DEFAULT '',
  deleted_at TEXT NOT NULL DEFAULT '',
  session_id TEXT NOT NULL DEFAULT '',
  run_id TEXT NOT NULL DEFAULT '',
  invocation_id TEXT NOT NULL DEFAULT '',
  agent_id TEXT NOT NULL DEFAULT '',
  team_id TEXT NOT NULL DEFAULT '',
  parent_trace_id TEXT NOT NULL DEFAULT '',
  duration_ms INTEGER NOT NULL DEFAULT 0,
  span_count INTEGER NOT NULL DEFAULT 0,
  error_count INTEGER NOT NULL DEFAULT 0,
  total_tokens INTEGER NOT NULL DEFAULT 0,
  total_cost_usd REAL NOT NULL DEFAULT 0.0
);

CREATE INDEX IF NOT EXISTS idx_monitor_traces_status ON monitor_traces(status);
CREATE INDEX IF NOT EXISTS idx_monitor_traces_created_at ON monitor_traces(created_at);
CREATE INDEX IF NOT EXISTS idx_monitor_traces_session_id ON monitor_traces(session_id);
CREATE INDEX IF NOT EXISTS idx_monitor_traces_run_id ON monitor_traces(run_id);
CREATE INDEX IF NOT EXISTS idx_monitor_traces_agent_id ON monitor_traces(agent_id);

CREATE TABLE IF NOT EXISTS monitor_trace_spans (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  trace_id TEXT NOT NULL,
  span_id TEXT NOT NULL,
  parent_span_id TEXT NOT NULL DEFAULT '',
  kind TEXT NOT NULL,
  name TEXT NOT NULL,
  started_at INTEGER NOT NULL,
  ended_at INTEGER NOT NULL DEFAULT 0,
  status TEXT NOT NULL DEFAULT 'running',
  attributes_json TEXT NOT NULL DEFAULT '',
  error_json TEXT NOT NULL DEFAULT '',
  UNIQUE(trace_id, span_id)
);

CREATE INDEX IF NOT EXISTS idx_monitor_trace_spans_trace_id ON monitor_trace_spans(trace_id);
CREATE INDEX IF NOT EXISTS idx_monitor_trace_spans_kind ON monitor_trace_spans(kind);
