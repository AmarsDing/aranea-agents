-- P2-B Phase 2: Add workspace_id to remaining tenant-owned platform tables.
-- Phase 1 (20261007) covered agents/teams/graph_definitions/plugins.
-- Ent schemas already declare these columns. Existing DBs need ALTER.
--
-- Semantics match 20261007:
--   * workspace_id = '' — shared / legacy (visible per AssertWorkspaceOrShared)
--   * workspace_id = 'ws-xxx' — tenant-private
--
-- DB-N6: statements are idempotent where the runner tolerates duplicate columns.

-- tools
ALTER TABLE tools ADD COLUMN workspace_id TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_tools_workspace ON tools(workspace_id, enabled);

-- skill
ALTER TABLE skill ADD COLUMN workspace_id TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_skill_workspace ON skill(workspace_id, enabled);

-- mcp_server
ALTER TABLE mcp_server ADD COLUMN workspace_id TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_mcp_server_workspace ON mcp_server(workspace_id, enabled);

-- channel
ALTER TABLE channel ADD COLUMN workspace_id TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_channel_workspace ON channel(workspace_id, enabled);

-- cron_task
ALTER TABLE cron_task ADD COLUMN workspace_id TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_cron_task_workspace ON cron_task(workspace_id, deleted_at);

-- eval_runs
ALTER TABLE eval_runs ADD COLUMN workspace_id TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_eval_runs_workspace ON eval_runs(workspace_id);

-- tasks_v2
ALTER TABLE tasks_v2 ADD COLUMN workspace_id TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_tasks_v2_workspace ON tasks_v2(workspace_id, status);

-- task_plans
ALTER TABLE task_plans ADD COLUMN workspace_id TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_task_plans_workspace ON task_plans(workspace_id, status);
