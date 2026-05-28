-- ============================================================
-- 索引: 所有表的索引集中管理
-- ============================================================

-- Session 索引
CREATE INDEX IF NOT EXISTS idx_sessions_agent ON sessions(agent_id, deleted_at, updated_at);
CREATE INDEX IF NOT EXISTS idx_sessions_team ON sessions(team_id, deleted_at, updated_at);
CREATE INDEX IF NOT EXISTS idx_sessions_last_message ON sessions(last_message_at);

-- Message 索引
CREATE INDEX IF NOT EXISTS idx_messages_session_turn ON messages(session_id, turn_number);

-- Session Summary 索引
CREATE INDEX IF NOT EXISTS idx_session_summaries_session_range ON session_summaries(session_id, to_turn);

-- Team 索引
CREATE INDEX IF NOT EXISTS idx_team_runs_team_created ON team_runs(team_id, created_at);
CREATE INDEX IF NOT EXISTS idx_team_runs_session ON team_runs(session_id, created_at);
CREATE INDEX IF NOT EXISTS idx_team_runs_trace ON team_runs(trace_id, created_at);
CREATE INDEX IF NOT EXISTS idx_orchestration_steps_run_created ON orchestration_steps(team_run_id, created_at);
CREATE INDEX IF NOT EXISTS idx_team_run_steps_run ON team_run_steps(run_id, sort_order);

-- Agent 索引
CREATE INDEX IF NOT EXISTS idx_agent_prompt_files_agent ON agent_prompt_files(agent_id, sort_order);
CREATE INDEX IF NOT EXISTS idx_avatar_assets_system ON avatar_assets(is_system, sort_order);
CREATE INDEX IF NOT EXISTS idx_avatar_assets_workspace_owner ON avatar_assets(workspace_id, owner_user_id);
CREATE INDEX IF NOT EXISTS idx_agent_category_parent ON agent_category_nodes(parent_id, sort_order);
CREATE INDEX IF NOT EXISTS idx_agent_category_level ON agent_category_nodes(level, sort_order);
CREATE INDEX IF NOT EXISTS idx_provider_models_provider ON llm_provider_models(provider, enabled, sort_order);

-- Channel 索引
CREATE INDEX IF NOT EXISTS idx_channel_delivery_channel ON channel_delivery(channel_id, created_at);
CREATE INDEX IF NOT EXISTS idx_hook_agents_agent ON hook_agents(agent_id, deleted_at);
CREATE INDEX IF NOT EXISTS idx_plugins_enabled_order ON plugins(enabled, sort_order);

-- Skill 索引
CREATE INDEX IF NOT EXISTS idx_skill_invocation_skill ON skill_invocation(skill_id, created_at);

-- Cron 索引
CREATE INDEX IF NOT EXISTS idx_cron_task_agent ON cron_task(agent_id, deleted_at);
CREATE INDEX IF NOT EXISTS idx_cron_run_task ON cron_task_run(task_id, created_at);

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
CREATE INDEX IF NOT EXISTS idx_pricing_rules_model_active ON model_pricing_rules(provider_code, model_api_id, is_active, effective_from);

-- Attachment 索引
CREATE INDEX IF NOT EXISTS idx_attachments_session ON chat_attachments(session_id, deleted_at);

-- Tool 索引
CREATE INDEX IF NOT EXISTS idx_tools_category ON tools(category);
CREATE INDEX IF NOT EXISTS idx_tools_source ON tools(source);
CREATE INDEX IF NOT EXISTS idx_tools_enabled ON tools(enabled);
CREATE INDEX IF NOT EXISTS idx_tools_risk_level ON tools(risk_level);
CREATE INDEX IF NOT EXISTS idx_tool_agent_overrides_agent ON tool_agent_overrides(agent_id);
CREATE INDEX IF NOT EXISTS idx_tool_agent_overrides_tool ON tool_agent_overrides(tool_key);
CREATE INDEX IF NOT EXISTS idx_tool_invocations_tool_time ON tool_invocations(tool_key, started_at);
CREATE INDEX IF NOT EXISTS idx_tool_invocations_agent_time ON tool_invocations(agent_id, started_at);
CREATE INDEX IF NOT EXISTS idx_tool_invocations_session ON tool_invocations(session_id);
CREATE INDEX IF NOT EXISTS idx_tool_invocations_status ON tool_invocations(status);
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
