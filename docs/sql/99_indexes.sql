-- ============================================================
-- 索引: 所有表的索引集中管理
-- ============================================================

-- Session 索引
-- [Ent-managed] idx_sessions_agent, idx_sessions_team, idx_sessions_last_message → ent/schema/session.go
-- [Ent-managed] idx_messages_session_turn → ent/schema/message.go

-- Session Summary 索引
CREATE INDEX IF NOT EXISTS idx_session_summaries_session_range ON session_summaries(session_id, to_turn);

-- Team 索引
-- [Ent-managed] idx_team_runs_team_created, idx_team_runs_session, idx_team_runs_trace → ent/schema/team_run.go
-- [Ent-managed] idx_team_run_steps_run → ent/schema/team_run_step.go
CREATE INDEX IF NOT EXISTS idx_orchestration_steps_run_created ON orchestration_steps(team_run_id, created_at);

-- Agent 索引
-- [Ent-managed] idx_agent_prompt_files_agent → ent/schema/agent_prompt_file.go
-- [Ent-managed] idx_avatar_assets_system, idx_avatar_assets_workspace_owner → ent/schema/avatar_asset.go
-- [Ent-managed] idx_agent_category_parent, idx_agent_category_level → ent/schema/agent_category.go
-- [Ent-managed] idx_provider_models_provider → ent/schema/llm_provider_model.go

-- Channel 索引
-- [Ent-managed] idx_channel_delivery_channel → ent/schema/platform_channel_delivery.go
-- [Ent-managed] idx_plugins_enabled_order → ent/schema/plugin.go
CREATE INDEX IF NOT EXISTS idx_hook_agents_agent ON hook_agents(agent_id, deleted_at);

-- Skill 索引
-- [Ent-managed] idx_skill_invocation_skill → ent/schema/skill_invocation.go

-- Cron 索引
-- [Ent-managed] idx_cron_task_agent → ent/schema/cron_task.go
-- [Ent-managed] idx_cron_run_task → ent/schema/cron_task_run.go

-- Monitor 索引
CREATE INDEX IF NOT EXISTS idx_monitor_events_created ON monitor_events(created_at);
CREATE INDEX IF NOT EXISTS idx_monitor_traces_created ON monitor_traces(created_at);

-- Usage 索引
CREATE INDEX IF NOT EXISTS idx_usage_events_time ON model_token_usage_events(occurred_at);
CREATE INDEX IF NOT EXISTS idx_usage_events_date_model ON model_token_usage_events(date_key, provider_code, model_api_id);
CREATE INDEX IF NOT EXISTS idx_usage_events_agent_time ON model_token_usage_events(agent_id, occurred_at);
CREATE INDEX IF NOT EXISTS idx_usage_events_session ON model_token_usage_events(session_id);
CREATE INDEX IF NOT EXISTS idx_usage_events_status ON model_token_usage_events(status, occurred_at);
CREATE INDEX IF NOT EXISTS idx_usage_daily_date_model ON model_token_usage_daily(date_key, provider_code, model_api_id);
CREATE INDEX IF NOT EXISTS idx_usage_hourly_hour ON model_token_usage_hourly(hour_key);
-- [Ent-managed] idx_pricing_rules_model_active → ent/schema/model_pricing_rule.go

-- Attachment 索引
CREATE INDEX IF NOT EXISTS idx_attachments_session ON chat_attachments(session_id, deleted_at);

-- Tool 索引
CREATE INDEX IF NOT EXISTS idx_tools_category ON tools(category);
CREATE INDEX IF NOT EXISTS idx_tools_source ON tools(source);
CREATE INDEX IF NOT EXISTS idx_tools_enabled ON tools(enabled);
CREATE INDEX IF NOT EXISTS idx_tools_risk_level ON tools(risk_level);
CREATE INDEX IF NOT EXISTS idx_tool_agent_overrides_agent ON tool_agent_overrides(agent_id);
CREATE INDEX IF NOT EXISTS idx_tool_agent_overrides_tool ON tool_agent_overrides(tool_key);
-- [Ent-managed] idx_tool_invocations_tool_time, idx_tool_invocations_agent_time, idx_tool_invocations_session, idx_tool_invocations_status → ent/schema/tool_invocation.go
CREATE INDEX IF NOT EXISTS idx_tool_invocation_params_invocation ON tool_invocation_params(invocation_id);
CREATE INDEX IF NOT EXISTS idx_tool_invocation_params_tool_param ON tool_invocation_params(tool_key, param_name);
CREATE INDEX IF NOT EXISTS idx_tool_usage_daily_tool_date ON tool_usage_daily(tool_key, date_key);
CREATE INDEX IF NOT EXISTS idx_tool_usage_daily_agent_date ON tool_usage_daily(agent_id, date_key);

-- Memory 通用索引
CREATE INDEX IF NOT EXISTS idx_memory_scope ON memory_items(scope_type, scope_id);

-- Memory L0 索引
CREATE INDEX IF NOT EXISTS idx_memory_l0_snapshots_session ON memory_l0_assembly_snapshots(session_id, created_at);
CREATE INDEX IF NOT EXISTS idx_memory_l0_snapshots_span ON memory_l0_assembly_snapshots(span_id);
CREATE INDEX IF NOT EXISTS idx_memory_l0_snapshots_agent ON memory_l0_assembly_snapshots(agent_id, created_at);

-- Memory L1 索引
CREATE INDEX IF NOT EXISTS idx_memory_l1_tasks_session ON memory_l1_tasks(session_id, status, updated_at);
CREATE INDEX IF NOT EXISTS idx_memory_l1_tasks_agent ON memory_l1_tasks(agent_id, status);
CREATE INDEX IF NOT EXISTS idx_memory_l1_fields_task ON memory_l1_fields(task_id, visibility, pin_to_prompt);
CREATE INDEX IF NOT EXISTS idx_memory_l1_fields_session ON memory_l1_fields(session_id, updated_at);
CREATE INDEX IF NOT EXISTS idx_memory_l1_field_history_field ON memory_l1_field_history(field_id, revision DESC);

-- Memory L2 索引
CREATE INDEX IF NOT EXISTS idx_memory_episodes_session ON memory_episodes(session_id, ended_at DESC);
CREATE INDEX IF NOT EXISTS idx_memory_episodes_agent ON memory_episodes(agent_id, importance DESC, ended_at DESC);
CREATE INDEX IF NOT EXISTS idx_memory_episodes_consolidation ON memory_episodes(consolidation_status, importance DESC, ended_at);
CREATE INDEX IF NOT EXISTS idx_memory_episodes_kind ON memory_episodes(episode_kind, ended_at DESC);
CREATE INDEX IF NOT EXISTS idx_memory_episodes_l1_task ON memory_episodes(l1_task_id);
CREATE INDEX IF NOT EXISTS idx_memory_l2_index_meta_episode ON memory_l2_index_meta(episode_id);
CREATE INDEX IF NOT EXISTS idx_memory_l2_index_meta_session_kind ON memory_l2_index_meta(session_id, text_kind);
CREATE INDEX IF NOT EXISTS idx_memory_event_marks_session ON memory_event_marks(session_id, mark_type, created_at);
CREATE INDEX IF NOT EXISTS idx_memory_event_marks_episode ON memory_event_marks(episode_id);

-- Memory L3 索引
CREATE INDEX IF NOT EXISTS idx_memory_facts_scope_status ON memory_facts(scope_type, scope_id, status, updated_at);
CREATE INDEX IF NOT EXISTS idx_memory_facts_workspace ON memory_facts(workspace_id, status, updated_at);
CREATE INDEX IF NOT EXISTS idx_memory_facts_agent ON memory_facts(agent_id, status, last_used_at);
CREATE INDEX IF NOT EXISTS idx_memory_facts_decay ON memory_facts(status, next_decay_at);
CREATE INDEX IF NOT EXISTS idx_memory_facts_kind ON memory_facts(fact_kind, scope_type, scope_id);
CREATE INDEX IF NOT EXISTS idx_memory_fact_versions_fact ON memory_fact_versions(fact_id, version DESC);
CREATE INDEX IF NOT EXISTS idx_memory_fact_feedback_fact ON memory_fact_feedback(fact_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_memory_fact_feedback_session ON memory_fact_feedback(session_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_memory_fact_conflicts_status ON memory_fact_conflicts(status, created_at);
CREATE INDEX IF NOT EXISTS idx_memory_fact_index_scope ON memory_fact_index(scope_type, scope_id);

-- Memory L4 索引
CREATE INDEX IF NOT EXISTS idx_memory_entities_scope_type ON memory_entities(scope_type, scope_id, entity_type, status);
CREATE INDEX IF NOT EXISTS idx_memory_entities_workspace ON memory_entities(workspace_id, entity_type, status);
CREATE INDEX IF NOT EXISTS idx_memory_entities_user ON memory_entities(user_id, entity_type, status);
CREATE INDEX IF NOT EXISTS idx_memory_relations_source ON memory_relations(source_id, status, weight DESC);
CREATE INDEX IF NOT EXISTS idx_memory_relations_target ON memory_relations(target_id, status, weight DESC);
CREATE INDEX IF NOT EXISTS idx_memory_relations_workspace ON memory_relations(workspace_id, status);
CREATE INDEX IF NOT EXISTS idx_memory_entity_facts_fact ON memory_entity_facts(fact_id);
CREATE INDEX IF NOT EXISTS idx_memory_entity_versions_entity ON memory_entity_versions(entity_id, version DESC);

-- Evolution 索引
CREATE INDEX IF NOT EXISTS idx_agent_evolution_events_agent ON agent_evolution_events(agent_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_agent_evolution_events_kind ON agent_evolution_events(agent_id, event_kind, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_agent_evolution_proposals_status ON agent_evolution_proposals(agent_id, status, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_agent_skill_stats_agent ON agent_skill_stats(agent_id, preference_score DESC);

-- Graph 索引
CREATE INDEX IF NOT EXISTS idx_graph_executions_graph ON graph_executions(graph_id);
CREATE INDEX IF NOT EXISTS idx_graph_executions_lineage ON graph_executions(lineage_id);
CREATE INDEX IF NOT EXISTS idx_graph_executions_status ON graph_executions(status);
CREATE INDEX IF NOT EXISTS idx_graph_tasks_execution ON graph_tasks(execution_id);
CREATE INDEX IF NOT EXISTS idx_graph_tasks_status ON graph_tasks(status);
CREATE INDEX IF NOT EXISTS idx_graph_tasks_assignee ON graph_tasks(assignee);
CREATE INDEX IF NOT EXISTS idx_graph_tasks_execution_status ON graph_tasks(execution_id, status);
CREATE INDEX IF NOT EXISTS idx_graph_task_comments_task ON graph_task_comments(task_id);
CREATE INDEX IF NOT EXISTS idx_graph_task_logs_task ON graph_task_logs(task_id);
CREATE INDEX IF NOT EXISTS idx_graph_task_logs_task_stream ON graph_task_logs(task_id, stream);
CREATE INDEX IF NOT EXISTS idx_graph_task_logs_task_level ON graph_task_logs(task_id, level);
CREATE INDEX IF NOT EXISTS idx_graph_task_runs_task ON graph_task_runs(task_id);
CREATE INDEX IF NOT EXISTS idx_graph_task_events_task ON graph_task_events(task_id);
CREATE INDEX IF NOT EXISTS idx_graph_task_events_type ON graph_task_events(event_type);
CREATE INDEX IF NOT EXISTS idx_graph_task_links_parent ON graph_task_links(parent_task_id);
CREATE INDEX IF NOT EXISTS idx_graph_task_links_child ON graph_task_links(child_task_id);
CREATE INDEX IF NOT EXISTS idx_graph_task_links_execution ON graph_task_links(execution_id);
