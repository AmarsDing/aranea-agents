-- Version 20261207: Agent Case 经验记忆（P3 M2 / EverOS Agent Memory 启发）。
-- User Memory（L3 facts）理解用户；Agent Case 理解任务。会话结束后由
-- AutoMemoryWorker 追加提取（复用 WriteL2Episode 门控），产出结构化经验
-- （goal/approach/outcome/pitfalls/tools_used），供 M3 召回注入与
-- M4 case→skill 蒸馏消费。
CREATE TABLE IF NOT EXISTS memory_agent_cases (
  id TEXT PRIMARY KEY,
  agent_id TEXT NOT NULL,
  user_id TEXT NOT NULL DEFAULT '',
  source_session_id TEXT NOT NULL,
  goal TEXT NOT NULL DEFAULT '',
  approach TEXT NOT NULL DEFAULT '',
  outcome TEXT NOT NULL DEFAULT 'partial',
  outcome_summary TEXT NOT NULL DEFAULT '',
  pitfalls TEXT NOT NULL DEFAULT '',
  tools_used TEXT NOT NULL DEFAULT '[]',
  quality REAL NOT NULL DEFAULT 0.5,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
-- 幂等锚点：同一会话重复提取/重试覆盖更新，不产生重复 Case。
CREATE UNIQUE INDEX IF NOT EXISTS idx_memory_agent_cases_agent_session ON memory_agent_cases(agent_id, source_session_id);
-- M3 召回按 agent + outcome 过滤。
CREATE INDEX IF NOT EXISTS idx_memory_agent_cases_agent_outcome ON memory_agent_cases(agent_id, outcome);
