-- ============================================================
-- Graph 工作流相关表: graph_definitions, graph_executions,
--                    graph_tasks, graph_task_comments,
--                    graph_task_logs, graph_task_runs,
--                    graph_task_events
-- ============================================================

CREATE TABLE IF NOT EXISTS graph_definitions (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL DEFAULT '',
  description TEXT NOT NULL DEFAULT '',
  state_fields TEXT NOT NULL DEFAULT '[]',
  nodes TEXT NOT NULL DEFAULT '[]',
  edges TEXT NOT NULL DEFAULT '[]',
  conditional_edges TEXT NOT NULL DEFAULT '[]',
  subgraphs TEXT NOT NULL DEFAULT '[]',
  entry_point TEXT NOT NULL DEFAULT '',
  finish_point TEXT NOT NULL DEFAULT '',
  enable_checkpoint INTEGER NOT NULL DEFAULT 0,
  execution_engine TEXT NOT NULL DEFAULT 'bsp',
  interrupt_before TEXT NOT NULL DEFAULT '[]',
  interrupt_after TEXT NOT NULL DEFAULT '[]',
  metadata TEXT NOT NULL DEFAULT '{}',
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS graph_executions (
  id TEXT PRIMARY KEY,
  graph_id TEXT NOT NULL DEFAULT '',
  session_id TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'running',
  current_node TEXT NOT NULL DEFAULT '',
  lineage_id TEXT NOT NULL DEFAULT '',
  error_message TEXT NOT NULL DEFAULT '',
  current_state_json TEXT NOT NULL DEFAULT '{}',
  steps_json TEXT NOT NULL DEFAULT '[]',
  started_at DATETIME NOT NULL,
  finished_at DATETIME
);

CREATE TABLE IF NOT EXISTS graph_tasks (
  id VARCHAR(64) PRIMARY KEY,
  node_id VARCHAR(128) NOT NULL DEFAULT '',
  execution_id VARCHAR(64) NOT NULL DEFAULT '',
  assignee VARCHAR(128) NOT NULL DEFAULT '',
  status VARCHAR(32) NOT NULL DEFAULT 'pending',
  context TEXT NOT NULL DEFAULT '{}',
  input TEXT NOT NULL DEFAULT '{}',
  output TEXT NOT NULL DEFAULT '',
  summary TEXT NOT NULL DEFAULT '',
  metadata TEXT NOT NULL DEFAULT '{}',
  required_role VARCHAR(128) NOT NULL DEFAULT '',
  assignment_mode VARCHAR(32) NOT NULL DEFAULT 'static',
  assignment_strategy VARCHAR(32) NOT NULL DEFAULT '',
  created_at DATETIME NOT NULL,
  claimed_at DATETIME,
  completed_at DATETIME
);

CREATE TABLE IF NOT EXISTS graph_task_comments (
  id VARCHAR(64) PRIMARY KEY,
  task_id VARCHAR(64) NOT NULL DEFAULT '',
  author VARCHAR(128) NOT NULL DEFAULT '',
  content TEXT NOT NULL DEFAULT '',
  type VARCHAR(32) NOT NULL DEFAULT 'suggestion',
  created_at DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS graph_task_logs (
  id VARCHAR(64) PRIMARY KEY,
  task_id VARCHAR(64) NOT NULL DEFAULT '',
  stream VARCHAR(16) NOT NULL DEFAULT 'stdout',
  content TEXT NOT NULL DEFAULT '',
  level VARCHAR(16) NOT NULL DEFAULT 'info',
  timestamp DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS graph_task_runs (
  id VARCHAR(64) PRIMARY KEY,
  task_id VARCHAR(64) NOT NULL DEFAULT '',
  started_at DATETIME NOT NULL,
  finished_at DATETIME,
  exit_code INTEGER NOT NULL DEFAULT 0,
  log_ref VARCHAR(256) NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS graph_task_events (
  id VARCHAR(64) PRIMARY KEY,
  task_id VARCHAR(64) NOT NULL DEFAULT '',
  event_type VARCHAR(64) NOT NULL DEFAULT '',
  source_node VARCHAR(128) NOT NULL DEFAULT '',
  description TEXT NOT NULL DEFAULT '',
  timestamp DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS graph_task_links (
  id VARCHAR(64) PRIMARY KEY,
  parent_task_id VARCHAR(64) NOT NULL DEFAULT '',
  child_task_id VARCHAR(64) NOT NULL DEFAULT '',
  execution_id VARCHAR(64) NOT NULL DEFAULT '',
  created_at DATETIME NOT NULL
);
