-- Version 20261249: memory_fact_pending（79-runtime-governance R3，记忆高风险写人工审批层）
-- 自动记忆管线 verdict 分流：UPDATE / DELETE / contested 判决不直写，落本表
-- status=pending，经审批中心（C7 桥 twinmonitor ai_approvals，source=memory_fact_write）
-- 裁决后执行原 bi-temporal 写或置 rejected 留痕。ADD / NOOP 不经过本表。
-- 双方言通用（SQLite 风格 DDL，PG 经 translateSQLiteDDLToPostgres 翻译）。
-- 幂等，重跑安全。
CREATE TABLE IF NOT EXISTS memory_fact_pending (
  id TEXT PRIMARY KEY,
  agent_id TEXT NOT NULL,
  fact_key TEXT NOT NULL DEFAULT '',
  verdict TEXT NOT NULL DEFAULT '',
  proposed_body TEXT NOT NULL DEFAULT '',
  prior_body TEXT NOT NULL DEFAULT '',
  adjudicator_reason TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'pending',
  approver TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL DEFAULT 0,
  decided_at INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_mfp_status
  ON memory_fact_pending(status, created_at);
CREATE INDEX IF NOT EXISTS idx_mfp_agent_status
  ON memory_fact_pending(agent_id, status);
