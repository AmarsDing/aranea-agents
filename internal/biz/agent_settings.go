package biz

// IdentityCfg holds routing/identity fields for an agent runtime.
type IdentityCfg struct {
	AgentID               string `json:"agent_id,omitempty"`
	ChannelID             string `json:"channel_id,omitempty"`
	ChatID                string `json:"chat_id,omitempty"`
	Workspace             string `json:"workspace,omitempty"`
	VariablesJSON         string `json:"variables_json,omitempty"`
	ModelInstructionsJSON string `json:"model_instructions_json,omitempty"`
}

// ReasoningCfg holds LLM reasoning strategy fields.
type ReasoningCfg struct {
	Mode  string `json:"reasoning_mode,omitempty"`
	Level string `json:"reasoning_level,omitempty"`
}

// MemoryCfg holds all memory + context-window settings (L0–L4).
type MemoryCfg struct {
	Enabled                  bool    `json:"memory_enabled,omitempty"`
	MaxChunkLength           int     `json:"memory_max_chunk_length,omitempty"`
	MaxResults               int     `json:"memory_max_results,omitempty"`
	MinScore                 float64 `json:"memory_min_score,omitempty"`
	HeartbeatEnabled         bool    `json:"heartbeat_enabled,omitempty"`
	HeartbeatIntervalMinutes int     `json:"heartbeat_interval_minutes,omitempty"`

	// L0 session-context window.
	L0RecentWindowTurns  int     `json:"l0_recent_window_turns,omitempty"`
	L0RecentWindowTokens int     `json:"l0_recent_window_tokens,omitempty"`
	L0SummaryThreshold   float64 `json:"l0_summary_threshold,omitempty"`
	L0SummaryKeepTurns   int     `json:"l0_summary_keep_turns,omitempty"`
	L0CompressProvider   string  `json:"l0_compress_provider,omitempty"`
	L0CompressModel      string  `json:"l0_compress_model,omitempty"`
	MemoryWorkerProvider string  `json:"memory_worker_provider,omitempty"`
	MemoryWorkerModel    string  `json:"memory_worker_model,omitempty"`
	L0TruncateStrategy   string  `json:"l0_truncate_strategy,omitempty"`
	L0InjectL1           bool    `json:"l0_inject_l1,omitempty"`
	L0InjectL3           bool    `json:"l0_inject_l3,omitempty"`
	L0InjectL4           bool    `json:"l0_inject_l4,omitempty"`
	L0L3MaxChunks        int     `json:"l0_l3_max_chunks,omitempty"`
	L0L4MaxPaths         int     `json:"l0_l4_max_paths,omitempty"`
	L0SnapshotMode       string  `json:"l0_snapshot_mode,omitempty"`

	// L1 working memory.
	L1Enabled              bool   `json:"l1_enabled,omitempty"`
	L1BudgetTokens         int    `json:"l1_budget_tokens,omitempty"`
	L1FieldMaxTokens       int    `json:"l1_field_max_tokens,omitempty"`
	L1HistoryKeepRevisions int    `json:"l1_history_keep_revisions,omitempty"`
	L1DefaultSchemaID      string `json:"l1_default_schema_id,omitempty"`
	L1ArchiveOnIdleMinutes int    `json:"l1_archive_on_idle_minutes,omitempty"`

	// L2 episodic memory.
	L2EpisodeEnabled       bool    `json:"l2_episode_enabled,omitempty"`
	L2EpisodeMinImportance float64 `json:"l2_episode_min_importance,omitempty"`
	L2IndexEnabled         bool    `json:"l2_index_enabled,omitempty"`
	L2IndexEmbeddingModel  string  `json:"l2_index_embedding_model,omitempty"`
	L2RecallEnabled        bool    `json:"l2_recall_enabled,omitempty"`
	L2RecallMax            int     `json:"l2_recall_max,omitempty"`
	L2RetentionDays        int     `json:"l2_retention_days,omitempty"`
	L2ArchiveAfterDays     int     `json:"l2_archive_after_days,omitempty"`

	// L3 semantic recall.
	L3Enabled            bool    `json:"l3_enabled,omitempty"`
	L3RecallTopK         int     `json:"l3_recall_top_k,omitempty"`
	L3RecallMinScore     float64 `json:"l3_recall_min_score,omitempty"`
	L3RecallScopesJSON   string  `json:"l3_recall_scopes_json,omitempty"`
	L3EmbeddingModel     string  `json:"l3_embedding_model,omitempty"`
	L3DecayIntervalHours int     `json:"l3_decay_interval_hours,omitempty"`
	L3ArchiveThreshold   float64 `json:"l3_archive_threshold,omitempty"`
	L3MaxPerRecallChars  int     `json:"l3_max_per_recall_chars,omitempty"`

	// L4 knowledge graph.
	L4Enabled              bool `json:"l4_enabled,omitempty"`
	L4GraphInjectNeighbors bool `json:"l4_graph_inject_neighbors,omitempty"`
	L4GraphMaxNeighbors    int  `json:"l4_graph_max_neighbors,omitempty"`
	L4GraphMaxHops         int  `json:"l4_graph_max_hops,omitempty"`
	L4IdentityInject       bool `json:"l4_identity_inject,omitempty"`
	L4StrategyInject       bool `json:"l4_strategy_inject,omitempty"`
}

// ToolsCfg holds tool execution and retry settings.
type ToolsCfg struct {
	Enabled                bool    `json:"tools_enabled,omitempty"`
	Profile                string  `json:"tools_profile,omitempty"`
	ToolCallPrefix         string  `json:"tools_tool_call_prefix,omitempty"`
	AllowJSON              string  `json:"tools_allow_json,omitempty"`
	DenyJSON               string  `json:"tools_deny_json,omitempty"`
	ConcurrentAllowJSON    string  `json:"tools_concurrent_allow_json,omitempty"`
	RetryEnabled           bool    `json:"tools_retry_enabled,omitempty"`
	RetryMaxAttempts       int     `json:"tools_retry_max_attempts,omitempty"`
	RetryInitialIntervalMs int     `json:"tools_retry_initial_interval_ms,omitempty"`
	RetryBackoffFactor     float64 `json:"tools_retry_backoff_factor,omitempty"`
	RetryMaxIntervalMs     int     `json:"tools_retry_max_interval_ms,omitempty"`
	RetryJitter            bool    `json:"tools_retry_jitter,omitempty"`
	ParallelEnabled        bool    `json:"tools_parallel_enabled,omitempty"`
	StreamingEnabled       bool    `json:"tools_streaming_enabled,omitempty"`
}

// SkillsCfg holds skill loading and intent-pass settings.
type SkillsCfg struct {
	RuntimeJSON       string `json:"skill_runtime_json,omitempty"`
	LoadMode          string `json:"skill_load_mode,omitempty"`
	IntentPassEnabled bool   `json:"intent_pass_enabled,omitempty"`
}

// CodeExecutorCfg holds per-agent code execution backend selection.
type CodeExecutorCfg struct {
	Type string `json:"code_executor_type,omitempty"` // local | docker | e2b | container
}

// PluginsCfg holds plugin runtime settings (reserved for future fields).
type PluginsCfg struct{}

// EvolutionCfg holds self-evolution, subagent, and guardrail settings.
type EvolutionCfg struct {
	SelfEvolve                        bool    `json:"self_evolve,omitempty"`
	SubagentsEnabled                  bool    `json:"subagents_enabled,omitempty"`
	SubagentsMaxConcurrency           int     `json:"subagents_max_concurrency,omitempty"`
	SubagentsMaxGenerationDepth       int     `json:"subagents_max_generation_depth,omitempty"`
	SubagentsMaxChildrenPerAgent      int     `json:"subagents_max_children_per_agent,omitempty"`
	SubagentsArchiveAfterMinutes      int     `json:"subagents_archive_after_minutes,omitempty"`
	SubagentsMaxRetries               int     `json:"subagents_max_retries,omitempty"`
	SubagentsModelOverride            string  `json:"subagents_model_override,omitempty"`
	SkillEvolve                       bool    `json:"evolution_skill_evolve,omitempty"`
	MetricsEnabled                    bool    `json:"evolution_metrics_enabled,omitempty"`
	SuggestionsEnabled                bool    `json:"evolution_suggestions_enabled,omitempty"`
	GuardrailMaxChangePerPeriod       float64 `json:"guardrail_max_change_per_period,omitempty"`
	GuardrailMinDataPoints            int     `json:"guardrail_min_data_points,omitempty"`
	GuardrailRollbackOnDeclinePercent int     `json:"guardrail_rollback_on_decline_percent,omitempty"`
	EvoEnabled                        bool    `json:"evo_enabled,omitempty"`
	EvoAutoApply                      bool    `json:"evo_auto_apply,omitempty"`
	EvoMinEpisodes                    int     `json:"evo_min_episodes,omitempty"`
	EvoMinNegativeFeedback            int     `json:"evo_min_negative_feedback,omitempty"`
	EvoThrottleHours                  int     `json:"evo_throttle_hours,omitempty"`
	EvoProposalTTLDays                int     `json:"evo_proposal_ttl_days,omitempty"`
	EvoPersonaMaxChars                int     `json:"evo_persona_max_chars,omitempty"`
	EvoSystemPromptMaxAppends         int     `json:"evo_system_prompt_max_appends,omitempty"`
}

// ContextCfg holds context-compaction, output-schema, model-selector, and planner settings.
type ContextCfg struct {
	CompactionEnabled     bool   `json:"context_compaction_enabled,omitempty"`
	SessionSummaryEnabled bool   `json:"session_summary_enabled,omitempty"`
	OutputSchemaJSON      string `json:"output_schema_json,omitempty"`
	ModelSelector         string `json:"model_selector,omitempty"`
	PlannerKind           string `json:"planner_kind,omitempty"`
	PlannerConfigJSON     string `json:"planner_config_json,omitempty"`
}
