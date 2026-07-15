-- P2-B Phase 1: Add workspace_id column to core tenant-owned entities.
-- Affected tables: agents, teams, graph_definitions, plugins.
--
-- Semantics:
--   * workspace_id = '' (empty) — shared / legacy / system-builtins (visible to all workspaces)
--   * workspace_id = 'ws-xxx'  — tenant-private (visible only to owning workspace)
--   * workspace_id = '__system__' — system sentinel (cron/admin bypass)
--
-- All existing rows default to '' so legacy data remains readable by all
-- workspaces until explicitly migrated. New tenant-scoped writes set the
-- caller workspace ID at the service layer.
--
-- DB-N6: each statement is idempotent (IF NOT EXISTS / duplicate-column tolerant).

-- agents
ALTER TABLE agents ADD COLUMN workspace_id TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_agents_workspace ON agents(workspace_id, deleted_at);

-- teams
ALTER TABLE teams ADD COLUMN workspace_id TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_teams_workspace ON teams(workspace_id, deleted_at);

-- graph_definitions
ALTER TABLE graph_definitions ADD COLUMN workspace_id TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_graph_definitions_workspace ON graph_definitions(workspace_id);

-- plugins
ALTER TABLE plugins ADD COLUMN workspace_id TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_plugins_workspace ON plugins(workspace_id, enabled);
