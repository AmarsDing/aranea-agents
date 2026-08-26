-- Version 20261250: decision_records（M80 决策智能 Phase 1，FR-80-01~03）
-- 统一决策记录层：五类高价值决策（HITL 人工审批 / planner 策略路由 / 系统闸动作 /
-- 知识仲裁裁决 / 进化建议应用）归一为可查询、可审计、可追溯的一等资产。
-- 单父链 parent_decision_id 指向本表 id（ADR-3）；related_entities/source_ref/metadata
-- 为 JSON 文本列（双方言惯例，同 attrs_json/evidence_json），PG 侧 GIN 索引由
-- 20261252 的 Func 门控迁移补（表达式索引 (col::jsonb)，SQLite 跳过）。
-- 双方言通用（SQLite 风格 DDL，PG 经 translateSQLiteDDLToPostgres 翻译：
-- INTEGER PRIMARY KEY AUTOINCREMENT -> BIGSERIAL PRIMARY KEY）。幂等，重跑安全。
CREATE TABLE IF NOT EXISTS decision_records (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  decision_key TEXT NOT NULL,
  category TEXT NOT NULL DEFAULT '',
  scenario TEXT NOT NULL DEFAULT '',
  reasoning TEXT NOT NULL DEFAULT '',
  outcome TEXT NOT NULL DEFAULT '',
  confidence DOUBLE PRECISION,
  actor_type TEXT NOT NULL DEFAULT '',
  actor_key TEXT NOT NULL DEFAULT '',
  parent_decision_id INTEGER,
  related_entities TEXT NOT NULL DEFAULT '[]',
  source_ref TEXT NOT NULL DEFAULT '{}',
  metadata TEXT NOT NULL DEFAULT '{}',
  workspace_id TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL DEFAULT ''
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_decision_records_key
  ON decision_records(decision_key);
