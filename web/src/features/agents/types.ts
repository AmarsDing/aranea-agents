export type AgentKind = '' | 'llm' | 'a2a_proxy';

export type AgentOwnership = '' | 'user' | 'system_builtin' | 'ecosystem_preset' | 'marketplace' | 'certified';

export type A2AProxyConfig = {
  remote_url: string;
  agent_card_url?: string;
  enable_streaming?: boolean;
  auth_type?: string;
  auth_config_json?: string;
  timeout_seconds?: number;
};

export type Agent = {
  id: string;
  agent_key: string;
  display_name: string;
  provider: string;
  model: string;
  agent_kind?: AgentKind;
  kind?: AgentOwnership; // ownership classification (user | system_builtin | ecosystem_preset | marketplace | certified)
  a2a_proxy_config?: A2AProxyConfig;
  a2a_endpoint_enabled?: boolean;
  last_run_status?: string;
  last_run_at?: string;
  pending_evolution_count?: number;
  status: string;
  is_default: boolean;
  is_favorite: boolean;
  icon: string;
  agent_description: string;
  position_key?: string;
  agent_variant?: string;
  variant_description?: string;
  taxonomy_position_id: string;
  system_prompt_mode: string;
  context_window: number;
  budget_monthly_cents: number;
  config_json: string;
  created_at: string;
  updated_at: string;
  deleted_at: string;
  created_by?: string;
  readonly?: boolean;
  source?: string; // user | system | imported (origin tracking)
  settings?: AgentRuntimeSettings;
  files?: AgentPromptFile[];
};

export type AgentRuntimeSettings = {
  agent_id?: string;
  self_evolve: boolean;
  subagents_enabled: boolean;
  subagents_max_concurrency: number;
  subagents_max_generation_depth: number;
  subagents_max_children_per_agent: number;
  subagents_archive_after_minutes: number;
  subagents_max_retries: number;
  subagents_stored_result_runes?: number;
  subagents_stored_summary_runes?: number;
  subagents_model_override: string;
  tools_enabled: boolean;
  tools_profile: string;
  tools_tool_call_prefix: string;
  tools_allow_json: string;
  tools_deny_json: string;
  tools_concurrent_allow_json: string;
  tools_retry_enabled?: boolean;
  tools_retry_max_attempts?: number;
  tools_retry_initial_interval_ms?: number;
  tools_retry_backoff_factor?: number;
  tools_retry_max_interval_ms?: number;
  tools_retry_jitter?: boolean;
  tools_parallel_enabled?: boolean;
  tools_streaming_enabled?: boolean;
  memory_enabled: boolean;
  memory_max_chunk_length: number;
  memory_max_results: number;
  memory_min_score: number;
  l0_recent_window_turns?: number;
  l0_recent_window_tokens?: number;
  l0_summary_threshold?: number;
  l0_summary_keep_turns?: number;
  l0_compress_provider?: string;
  l0_compress_model?: string;
  memory_worker_provider?: string;
  memory_worker_model?: string;
  l0_truncate_strategy?: string;
  l0_inject_l1?: boolean;
  l0_inject_l3?: boolean;
  l0_inject_l4?: boolean;
  l0_l3_max_chunks?: number;
  l0_l4_max_paths?: number;
  l0_snapshot_mode?: string;
  l1_enabled?: boolean;
  l1_budget_tokens?: number;
  l1_field_max_tokens?: number;
  l1_history_keep_revisions?: number;
  l1_default_schema_id?: string;
  l1_archive_on_idle_minutes?: number;
  l2_episode_enabled?: boolean;
  l2_episode_min_importance?: number;
  l2_index_enabled?: boolean;
  l2_index_embedding_model?: string;
  l2_recall_enabled?: boolean;
  l2_recall_max?: number;
  l2_retention_days?: number;
  l2_archive_after_days?: number;
  l3_enabled?: boolean;
  l3_recall_top_k?: number;
  l3_recall_min_score?: number;
  l3_recall_scopes_json?: string;
  l3_embedding_model?: string;
  l3_decay_interval_hours?: number;
  l3_archive_threshold?: number;
  l3_max_per_recall_chars?: number;
  l4_enabled?: boolean;
  l4_graph_inject_neighbors?: boolean;
  l4_graph_max_neighbors?: number;
  l4_graph_max_hops?: number;
  l4_identity_inject?: boolean;
  l4_strategy_inject?: boolean;
  evo_enabled?: boolean;
  evo_auto_apply?: boolean;
  evo_min_episodes?: number;
  evo_min_negative_feedback?: number;
  evo_throttle_hours?: number;
  evo_proposal_ttl_days?: number;
  evo_persona_max_chars?: number;
  evo_system_prompt_max_appends?: number;
  heartbeat_enabled: boolean;
  heartbeat_interval_minutes: number;
  evolution_self_evolve: boolean;
  evolution_skill_evolve: boolean;
  evolution_metrics_enabled: boolean;
  evolution_suggestions_enabled: boolean;
  guardrail_max_change_per_period: number;
  guardrail_min_data_points: number;
  guardrail_rollback_on_decline_percent: number;
  skill_runtime_json?: string;
  intent_pass_enabled?: boolean;
  variables_json?: string;
  model_instructions_json?: string;
  context_compaction_enabled?: boolean;
  session_summary_enabled?: boolean;
  skill_load_mode?: string;
  code_executor_type?: string;
  output_schema_json?: string;
  model_selector?: string;
  channel_id?: string;
  chat_id?: string;
  workspace?: string;
  reasoning_mode?: string;
  reasoning_level?: string;
  planner_kind?: string;
  planner_config_json?: string;
  ralph_loop_max_iterations?: number;
  ralph_loop_completion_promise?: string;
  ralph_loop_verify_command?: string;
  ralph_loop_verify_timeout_seconds?: number;
  ralph_loop_promise_tag_open?: string;
  ralph_loop_promise_tag_close?: string;
  ralph_loop_verify_work_dir?: string;
  micro_compact_enabled?: boolean;
  memory_compact_enabled?: boolean;
  tool_result_gate_enabled?: boolean;
  tools_circuit_breaker_enabled?: boolean;
  tools_circuit_breaker_overrides_json?: string;
  tools_deferred_json?: string;
  tools_command_safety_enabled?: boolean;
  compress_llm_cache_enabled?: boolean;
  compress_llm_cache_max_entries?: number;
  compress_llm_cache_ttl_sec?: number;
  verification_truncate_chars?: number;
  created_at?: string;
  updated_at?: string;
};

export type AgentPromptFile = {
  id?: string;
  agent_id?: string;
  name: string;
  body: string;
  sort_order: number;
  created_at?: string;
  updated_at?: string;
};

export type AgentListQuery = {
  keyword?: string;
  status?: string;
  provider?: string;
  /** S-10 fix: renamed from category_id to match proto field org_node_id */
  org_node_id?: string;
  /** empty = all; "mine" = current user; otherwise user id */
  created_by?: string;
  limit?: number;
  offset?: number;
};

export type AgentCreatorOption = {
  user_id: string;
  label: string;
};

export type AgentTemplatePreset = {
  key: string;
  label: string;
  icon: string;
  description: string;
  text?: string;
  display_name?: string;
  provider?: string;
  model?: string;
};

export type AgentListResult = {
  items: Agent[];
  total: number;
  limit: number;
  offset: number;
};

export type MetricDataPoint = {
  date: string;
  value: number;
};

export type EvolutionMetrics = {
  agent_id: string;
  time_range: string;
  tool_success_rate: number;
  retrieval_quality: number;
  total_episodes: number;
  negative_feedback: number;
  tool_success_series: MetricDataPoint[];
  retrieval_quality_series: MetricDataPoint[];
};

export type EvolutionSuggestion = {
  id: string;
  agent_id: string;
  type: string;
  title: string;
  content: string;
  status: string;
  diff_preview: string;
  created_at: string;
  applied_at: string;
};

export type AgentPromptSection = {
  key: string;
  label: string;
  est_tokens: number;
  source: string;
};

export type AgentPromptPreview = {
  summary: string;
  instruction: string;
  sections: AgentPromptSection[];
  static_total_tokens: number;
  runtime_overlay_est_tokens: number;
  runtime_note: string;
};
