-- Version 20261256: memory_fact_allow_rules（79-runtime-governance R3 Phase 3.4，E4）
-- 记忆高风险写审批四档决议与工具 HITL 对齐：approve_always 以 (agent_id, verdict)
-- 维度持久化 allow 规则，命中后同 agent 同类 verdict 免审直写（仍记 audit）。
-- approve_session 为进程内会话级授权（不落本表）。双方言通用（SQLite 风格 DDL，
-- PG 经 translateSQLiteDDLToPostgres 翻译）。幂等，重跑安全。
CREATE TABLE IF NOT EXISTS memory_fact_allow_rules (
  id TEXT PRIMARY KEY,
  agent_id TEXT NOT NULL,
  verdict TEXT NOT NULL DEFAULT '',
  created_by TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL DEFAULT 0
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_mfar_agent_verdict
  ON memory_fact_allow_rules(agent_id, verdict);
