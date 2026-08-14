-- 进化建议模块 P0-1a（IDOR 修复）：unified_evolution_suggestions 工作区隔离。
-- empty workspace_id = 共享/平台级行（所有租户可见；存量无法定位宿主的行降级为此语义）；
-- non-empty = 租户私有行（仅归属租户可见）。
-- backfill：skill 目标取 skill.workspace_id，agent 目标取 agents.workspace_id；
-- 宿主已删除/不存在的行保持 ''（共享可见，不丢数据）。
-- RLS 与 20261011_tenant_rls_phase1 同策略（ENABLE only，无 FORCE，GUC 未接线前不改变行为）。
-- Postgres-only：经 ddlUnifiedEvolutionWorkspace 挂载（非 Postgres 方言跳过）。

ALTER TABLE unified_evolution_suggestions ADD COLUMN IF NOT EXISTS workspace_id TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_ues_workspace ON unified_evolution_suggestions(workspace_id);

UPDATE unified_evolution_suggestions u SET workspace_id = s.workspace_id
FROM skill s
WHERE u.target_type = 'skill' AND u.target_id = s.id AND u.workspace_id = '';

UPDATE unified_evolution_suggestions u SET workspace_id = a.workspace_id
FROM agents a
WHERE u.target_type = 'agent' AND u.target_id = a.id AND u.workspace_id = '';

ALTER TABLE unified_evolution_suggestions ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_workspace_isolation ON unified_evolution_suggestions;
CREATE POLICY tenant_workspace_isolation ON unified_evolution_suggestions
  USING (
    workspace_id = current_setting('app.workspace_id', true)
    OR workspace_id = ''
    OR current_setting('app.workspace_id', true) IN ('', '__system__')
  );
