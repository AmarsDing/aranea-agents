-- Planner: per-agent planning strategy and JSON config (builtin / react / a2ui)
-- See docs/需求/39-planner-development.md; legacy cleanup: 02_agent_planner_legacy_cleanup.sql
ALTER TABLE agent_runtime_settings ADD COLUMN planner_kind TEXT NOT NULL DEFAULT '';
ALTER TABLE agent_runtime_settings ADD COLUMN planner_config_json TEXT NOT NULL DEFAULT '{}';
