package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
)

// AgentRuntimeSetting maps legacy table agent_runtime_settings.
type AgentRuntimeSetting struct {
	ent.Schema
}

func (AgentRuntimeSetting) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "agent_runtime_settings"},
	}
}

func (AgentRuntimeSetting) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").StorageKey("agent_id").Unique().Immutable().MaxLen(256),
		field.Bool("self_evolve").Default(true),
		field.Bool("subagents_enabled").Default(true),
		field.Int("subagents_max_concurrency").Default(20),
		field.Int("subagents_max_generation_depth").Default(1),
		field.Int("subagents_max_children_per_agent").Default(5),
		field.Int("subagents_archive_after_minutes").Default(60),
		field.Int("subagents_max_retries").Default(2),
		field.String("subagents_model_override").Default(""),
		field.Int("subagents_stored_result_runes").Default(4000),
		field.Int("subagents_stored_summary_runes").Default(240),
		field.Bool("tools_enabled").Default(true),
		field.String("tools_profile").Default("coding"),
		field.String("tools_tool_call_prefix").Default(""),
		field.String("tools_allow_json").Default("[]"),
		field.String("tools_deny_json").Default("[]"),
		field.String("tools_concurrent_allow_json").Default("[]"),
		field.Bool("memory_enabled").Default(true),
		field.Int("memory_max_chunk_length").Default(1000),
		field.Int("memory_max_results").Default(6),
		field.Float("memory_min_score").Default(0.35),
		field.Bool("heartbeat_enabled").Default(false),
		field.Int("heartbeat_interval_minutes").Default(30),
		field.Bool("evolution_self_evolve").Default(true),
		field.Bool("evolution_skill_evolve").Default(true),
		field.Bool("evolution_metrics_enabled").Default(true),
		field.Bool("evolution_suggestions_enabled").Default(true),
		field.Float("guardrail_max_change_per_period").Default(0.1),
		field.Int("guardrail_min_data_points").Default(100),
		field.Int("guardrail_rollback_on_decline_percent").Default(20),
		field.Int("l0_recent_window_turns").Default(12),
		field.Int("l0_recent_window_tokens").Default(0),
		field.Float("l0_summary_threshold").Default(0.6),
		field.Int("l0_summary_keep_turns").Default(4),
		field.Int("l0_compress_min_gap_sec").Default(600),
		field.String("l0_compress_provider").Default(""),
		field.String("l0_compress_model").Default(""),
		field.String("memory_worker_provider").Default(""),
		field.String("memory_worker_model").Default(""),
		field.String("l0_truncate_strategy").Default("summary"),
		field.Bool("l0_inject_l1").Default(true),
		field.Bool("l0_inject_l3").Default(true),
		field.Bool("l0_inject_l4").Default(true),
		field.Int("l0_l3_max_chunks").Default(5),
		field.Int("l0_l4_max_paths").Default(3),
		field.String("l0_snapshot_mode").Default("on_warning"),
		field.Bool("l0_snapshot_enabled").Default(true),
		field.Bool("l1_enabled").Default(true),
		field.Int("l1_budget_tokens").Default(8192),
		field.Int("l1_field_max_tokens").Default(2048),
		field.Int("l1_history_keep_revisions").Default(10),
		field.Bool("l1_history_enabled").Default(false),
		field.String("l1_default_schema_id").Default(""),
		field.Int("l1_archive_on_idle_minutes").Default(60),
		field.Bool("l2_episode_enabled").Default(true),
		field.Float("l2_episode_min_importance").Default(0.3),
		field.Bool("l2_index_enabled").Default(true),
		field.String("l2_index_embedding_model").Default(""),
		field.Bool("l2_recall_enabled").Default(true),
		field.Int("l2_recall_max").Default(3),
		field.Int("l2_retention_days").Default(90),
		field.Int("l2_archive_after_days").Default(30),
		field.Bool("l3_enabled").Default(true),
		field.Int("l3_recall_top_k").Default(5),
		field.Float("l3_recall_min_score").Default(0.35),
		field.String("l3_recall_scopes_json").Default("[\"agent\",\"user\",\"team\",\"workspace\"]"),
		field.String("l3_embedding_model").Default(""),
		field.Int("l3_decay_interval_hours").Default(24),
		field.Float("l3_archive_threshold").Default(0.2),
		field.Int("l3_max_per_recall_chars").Default(1500),
		// FR-12/P2: 召回块 token 预算（档位 400/800/1600，默认 800）。
		field.Int("l3_recall_budget_tokens").Default(800),
		field.Bool("l4_enabled").Default(true),
		field.Bool("l4_graph_inject_neighbors").Default(true),
		field.Int("l4_graph_max_neighbors").Default(6),
		field.Int("l4_graph_max_hops").Default(2),
		field.Bool("l4_identity_inject").Default(true),
		field.Bool("l4_strategy_inject").Default(false),
		field.Int("l4_decay_interval_hours").Default(48),
		field.String("l4_decay_overrides_json").Default("{}"),
		field.Bool("evo_enabled").Default(false),
		field.Bool("evo_auto_apply").Default(false),
		field.Int("evo_min_episodes").Default(20),
		field.Int("evo_min_negative_feedback").Default(3),
		field.Int("evo_throttle_hours").Default(24),
		field.Int("evo_proposal_ttl_days").Default(14),
		field.Int("evo_persona_max_chars").Default(1500),
		field.Int("evo_system_prompt_max_appends").Default(5),
		// JSON policy for narrow Skill toolset per agent: allowed_slugs, denied_slugs, allowed_tags, intent routing caps (see docs/需求/20 skill struct design.md 十三′).
		field.String("skill_runtime_json").Default("{}"),
		// Pre-main LLM pass to classify/refine user intent (extra latency/cost); persisted per agent; env ARANEA_INTENT_PASS can override.
		// P1-1 default ON: aligns with DDL migration (sql/migrations/20260607_agent_runtime_patches.sql:8) and DefaultAgentRuntimeSettings.
		field.Bool("intent_pass_enabled").Default(true),
		// Clarification gate: when intent pass detects blocking ambiguity, ask the user
		// paginated clarification questions before planning (P-CLARIFY, B.10.18). Default ON.
		field.Bool("clarification_enabled").Default(true),
		field.String("channel_id").Default(""),
		field.String("chat_id").Default(""),
		field.String("workspace").Default(""),
		field.String("reasoning_mode").Default("provider_default"),
		field.String("reasoning_level").Default("off"),
		field.String("variables_json").Default("{}"),
		field.String("model_instructions_json").Default("{}"),
		field.Bool("context_compaction_enabled").Default(true),
		field.Bool("memory_compact_enabled").Default(true),
		field.Bool("tool_result_gate_enabled").Default(true),
		field.Bool("compress_llm_cache_enabled").Default(true),
		field.Int("compress_llm_cache_max_entries").Default(256),
		field.Int("compress_llm_cache_ttl_sec").Default(600),
		// Token tailoring (provider-level context window management).
		field.Bool("enable_token_tailoring").Default(false),
		field.String("token_tailoring_strategy").Default(""),
		field.Float("token_tailoring_safety_margin").Default(0.0),
		field.Float("compression_buffer_ratio").Default(0.15),
		// CompressionBufferAdaptive enables adaptive buffer ratio adjustment based on token increment patterns.
		field.Bool("compression_buffer_adaptive").Default(true),
		field.Float("soft_trigger_ratio").Default(0.70),
		field.Float("hard_trigger_ratio").Default(0.90),
		field.Bool("session_summary_enabled").Default(true),
		field.String("skill_load_mode").Default("auto"),
		field.String("code_executor_type").Default("local"),
		// MaxLLMCalls limits LLM calls per turn (0 = unlimited); maps to llmagent.WithMaxLLMCalls.
		field.Int("max_llm_calls").Default(0),
		// MaxToolIterations limits tool-call iterations per turn (0 = unlimited); maps to llmagent.WithMaxToolIterations.
		field.Int("max_tool_iterations").Default(0),
		field.String("planner_kind").Default(""),
		field.String("planner_config_json").Default("{}"),
		field.Int("ralph_loop_max_iterations").Default(0),
		field.String("ralph_loop_completion_promise").Default(""),
		field.String("ralph_loop_verify_command").Default(""),
		field.Int("ralph_loop_verify_timeout_seconds").Default(0),
		field.String("ralph_loop_promise_tag_open").Default(""),
		field.String("ralph_loop_promise_tag_close").Default(""),
		field.String("ralph_loop_verify_work_dir").Default(""),
		field.String("output_schema_json").Default(""),
		field.String("model_selector").Default("default"),
		field.Bool("tools_retry_enabled").Default(false),
		field.Int("tools_retry_max_attempts").Default(2),
		field.Int("tools_retry_initial_interval_ms").Default(500),
		field.Float("tools_retry_backoff_factor").Default(2.0),
		field.Int("tools_retry_max_interval_ms").Default(5000),
		field.Bool("tools_retry_jitter").Default(true),
		field.Bool("tools_parallel_enabled").Default(false),
		field.Bool("tools_streaming_enabled").Default(false),
		field.Bool("tools_circuit_breaker_enabled").Default(false),
		field.String("tools_circuit_breaker_overrides_json").Default(""),
		field.String("tools_deferred_json").Default(""),
		field.Bool("tools_command_safety_enabled").Default(false),
		// ToolsExecutionTimeoutSec is the per-tool execution timeout in seconds (0 = use default safety-net).
		field.Int("tools_execution_timeout_sec").Default(0),
		// ForgetPolicyJSON stores the memory butler's forget policy configuration.
		field.String("forget_policy_json").Default("{}"),
		// ToolWeightJSON stores tool weight analysis results for prompt priority hints.
		field.String("tool_weight_json").Default("{}"),
		// DreamSnapshotJSON stores dream_cycle execution snapshots for rollback.
		field.String("dream_snapshot_json").Default(""),
		field.Int("verification_truncate_chars").Default(2000),
		field.String("created_at").Default(""),
		field.String("updated_at").Default(""),
	}
}
