-- Version 20261232: Agent runtime settings — token 成本审查（2026-08-20）补列
-- 方案A：l2_recall_budget_tokens，L2 独立召回预算（此前复用 l3_recall_budget_tokens）。
-- 方案B：l3_inject_provenance，L3 事实 provenance 注入开关（此前硬编码 true）。
-- Idempotent: "duplicate column" errors are treated as success by the migration runner (DB-N6).
ALTER TABLE agent_runtime_settings ADD COLUMN l2_recall_budget_tokens INTEGER NOT NULL DEFAULT 800;
ALTER TABLE agent_runtime_settings ADD COLUMN l3_inject_provenance INTEGER NOT NULL DEFAULT 1;
