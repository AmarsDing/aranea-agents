-- C-25 Phase 1: Postgres Row Level Security on tenant-owned tables.
-- ENABLE only (no FORCE) so table owners / migration roles still bypass RLS
-- until app.workspace_id GUC wiring is complete and verified.
--
-- Policy: row visible when
--   workspace_id = current_setting('app.workspace_id', true)
--   OR workspace_id = ''                         -- shared / legacy
--   OR current_setting IN ('', '__system__')      -- unset or system bypass
--
-- Real table names (Ent annotations), not logical "platform_*" names:
--   agents, teams, mcp_server, tools, skill, channel, cron_task, task_plans, tasks_v2
--
-- Postgres-only. Applied via ddlTenantRLSPhase1 (skipped on SQLite).

ALTER TABLE agents ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_workspace_isolation ON agents;
CREATE POLICY tenant_workspace_isolation ON agents
  USING (
    workspace_id = current_setting('app.workspace_id', true)
    OR workspace_id = ''
    OR current_setting('app.workspace_id', true) IN ('', '__system__')
  );

ALTER TABLE teams ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_workspace_isolation ON teams;
CREATE POLICY tenant_workspace_isolation ON teams
  USING (
    workspace_id = current_setting('app.workspace_id', true)
    OR workspace_id = ''
    OR current_setting('app.workspace_id', true) IN ('', '__system__')
  );

ALTER TABLE mcp_server ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_workspace_isolation ON mcp_server;
CREATE POLICY tenant_workspace_isolation ON mcp_server
  USING (
    workspace_id = current_setting('app.workspace_id', true)
    OR workspace_id = ''
    OR current_setting('app.workspace_id', true) IN ('', '__system__')
  );

ALTER TABLE tools ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_workspace_isolation ON tools;
CREATE POLICY tenant_workspace_isolation ON tools
  USING (
    workspace_id = current_setting('app.workspace_id', true)
    OR workspace_id = ''
    OR current_setting('app.workspace_id', true) IN ('', '__system__')
  );

ALTER TABLE skill ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_workspace_isolation ON skill;
CREATE POLICY tenant_workspace_isolation ON skill
  USING (
    workspace_id = current_setting('app.workspace_id', true)
    OR workspace_id = ''
    OR current_setting('app.workspace_id', true) IN ('', '__system__')
  );

ALTER TABLE channel ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_workspace_isolation ON channel;
CREATE POLICY tenant_workspace_isolation ON channel
  USING (
    workspace_id = current_setting('app.workspace_id', true)
    OR workspace_id = ''
    OR current_setting('app.workspace_id', true) IN ('', '__system__')
  );

ALTER TABLE cron_task ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_workspace_isolation ON cron_task;
CREATE POLICY tenant_workspace_isolation ON cron_task
  USING (
    workspace_id = current_setting('app.workspace_id', true)
    OR workspace_id = ''
    OR current_setting('app.workspace_id', true) IN ('', '__system__')
  );

ALTER TABLE task_plans ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_workspace_isolation ON task_plans;
CREATE POLICY tenant_workspace_isolation ON task_plans
  USING (
    workspace_id = current_setting('app.workspace_id', true)
    OR workspace_id = ''
    OR current_setting('app.workspace_id', true) IN ('', '__system__')
  );

ALTER TABLE tasks_v2 ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_workspace_isolation ON tasks_v2;
CREATE POLICY tenant_workspace_isolation ON tasks_v2
  USING (
    workspace_id = current_setting('app.workspace_id', true)
    OR workspace_id = ''
    OR current_setting('app.workspace_id', true) IN ('', '__system__')
  );
