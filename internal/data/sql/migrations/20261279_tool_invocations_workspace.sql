-- A2（tools 端到端审计跟进 2026-10-19）：tool_invocations 工作区化。
-- empty workspace_id = 宿主 agents 行缺失/无法归属的存量行（仅系统调用方在 KPI 聚合中可见）；
-- non-empty = 归属租户行（tenant 视角 KPI 仅统计本租户）。
-- backfill：按 agent_id 自 agents 表派生；宿主已删除的行保持 ''（不丢数据）。
-- SQLite 侧由 Ent schema 建列（CLI/测试路径）；本文件 Postgres-only，经
-- ddlToolInvocationsWorkspace 挂载（同 20261212 / ddlToolGrantsExpiresAt 惯例）。
-- 不加 RLS：本表租户边界一贯由 service 层断言（ListToolRuns workspace 过滤），
-- 未纳入 20261011 phase1 RLS 范围，A2 不扩大爆炸半径。

ALTER TABLE tool_invocations ADD COLUMN IF NOT EXISTS workspace_id TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_tool_invocations_ws_time ON tool_invocations(workspace_id, started_at);

UPDATE tool_invocations ti SET workspace_id = a.workspace_id
FROM agents a
WHERE ti.agent_id = a.id AND ti.workspace_id = '';
