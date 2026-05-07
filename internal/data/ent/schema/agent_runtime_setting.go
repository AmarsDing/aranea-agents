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
		field.Bool("tools_enabled").Default(true),
		field.String("tools_profile").Default("full"),
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
		field.String("l0_truncate_strategy").Default("summary"),
		field.Bool("l0_inject_l1").Default(true),
		field.Bool("l0_inject_l3").Default(true),
		field.Bool("l0_inject_l4").Default(false),
		field.Int("l0_l3_max_chunks").Default(5),
		field.Int("l0_l4_max_paths").Default(3),
		field.String("l0_snapshot_mode").Default("on_warning"),
		field.Bool("l1_enabled").Default(true),
		field.Int("l1_budget_tokens").Default(8192),
		field.Int("l1_field_max_tokens").Default(2048),
		field.Int("l1_history_keep_revisions").Default(10),
		field.String("l1_default_schema_id").Default(""),
		field.Int("l1_archive_on_idle_minutes").Default(60),
		field.Bool("l2_episode_enabled").Default(true),
		field.Float("l2_episode_min_importance").Default(0.3),
		field.Bool("l2_index_enabled").Default(true),
		field.String("l2_index_embedding_model").Default(""),
		field.Bool("l2_recall_enabled").Default(false),
		field.Int("l2_recall_max").Default(3),
		field.Int("l2_retention_days").Default(90),
		field.Int("l2_archive_after_days").Default(30),
		field.Bool("l3_enabled").Default(true),
		field.Int("l3_recall_top_k").Default(5),
		field.Float("l3_recall_min_score").Default(0.55),
		field.String("l3_recall_scopes_json").Default("[\"agent\",\"user\",\"team\",\"workspace\"]"),
		field.String("l3_embedding_model").Default(""),
		field.Int("l3_decay_interval_hours").Default(24),
		field.Float("l3_archive_threshold").Default(0.2),
		field.Int("l3_max_per_recall_chars").Default(1500),
		field.Bool("l4_enabled").Default(true),
		field.Bool("l4_graph_inject_neighbors").Default(true),
		field.Int("l4_graph_max_neighbors").Default(6),
		field.Int("l4_graph_max_hops").Default(2),
		field.Bool("l4_identity_inject").Default(true),
		field.Bool("l4_strategy_inject").Default(false),
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
		field.String("created_at").Default(""),
		field.String("updated_at").Default(""),
	}
}
