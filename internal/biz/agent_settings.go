package biz

import (
	"strings"
)

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
	L0CompressMinGapSec  int     `json:"l0_compress_min_gap_sec,omitempty"`
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
	L0SnapshotEnabled    bool    `json:"l0_snapshot_enabled,omitempty"`

	// L1 working memory.
	L1Enabled              bool   `json:"l1_enabled,omitempty"`
	L1BudgetTokens         int    `json:"l1_budget_tokens,omitempty"`
	L1FieldMaxTokens       int    `json:"l1_field_max_tokens,omitempty"`
	L1HistoryKeepRevisions int    `json:"l1_history_keep_revisions,omitempty"`
	L1HistoryEnabled       bool   `json:"l1_history_enabled,omitempty"`
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
	L3RecallBudgetTokens int     `json:"l3_recall_budget_tokens,omitempty"`

	// L4 knowledge graph.
	L4Enabled              bool   `json:"l4_enabled,omitempty"`
	L4GraphInjectNeighbors bool   `json:"l4_graph_inject_neighbors,omitempty"`
	L4GraphMaxNeighbors    int    `json:"l4_graph_max_neighbors,omitempty"`
	L4GraphMaxHops         int    `json:"l4_graph_max_hops,omitempty"`
	L4IdentityInject       bool   `json:"l4_identity_inject,omitempty"`
	L4StrategyInject       bool   `json:"l4_strategy_inject,omitempty"`
	L4DecayIntervalHours   int    `json:"l4_decay_interval_hours,omitempty"`
	L4DecayOverridesJSON   string `json:"l4_decay_overrides_json,omitempty"`

	// ForgetConfigJSON stores the memory butler's forget policy configuration.
	ForgetConfigJSON string `json:"forget_config_json,omitempty"`
}

// ToolsCfg holds tool execution and retry settings.
type ToolsCfg struct {
	Enabled                     bool    `json:"tools_enabled,omitempty"`
	Profile                     string  `json:"tools_profile,omitempty"`
	ToolCallPrefix              string  `json:"tools_tool_call_prefix,omitempty"`
	AllowJSON                   string  `json:"tools_allow_json,omitempty"`
	DenyJSON                    string  `json:"tools_deny_json,omitempty"`
	ConcurrentAllowJSON         string  `json:"tools_concurrent_allow_json,omitempty"`
	RetryEnabled                bool    `json:"tools_retry_enabled,omitempty"`
	RetryMaxAttempts            int     `json:"tools_retry_max_attempts,omitempty"`
	RetryInitialIntervalMs      int     `json:"tools_retry_initial_interval_ms,omitempty"`
	RetryBackoffFactor          float64 `json:"tools_retry_backoff_factor,omitempty"`
	RetryMaxIntervalMs          int     `json:"tools_retry_max_interval_ms,omitempty"`
	RetryJitter                 bool    `json:"tools_retry_jitter,omitempty"`
	ParallelEnabled             bool    `json:"tools_parallel_enabled,omitempty"`
	StreamingEnabled            bool    `json:"tools_streaming_enabled,omitempty"`
	CircuitBreakerEnabled       bool    `json:"tools_circuit_breaker_enabled,omitempty"`
	CircuitBreakerOverridesJSON string  `json:"tools_circuit_breaker_overrides_json,omitempty"`
	CommandSafetyEnabled        bool    `json:"tools_command_safety_enabled,omitempty"`
	ExecutionTimeoutSec         int     `json:"tools_execution_timeout_sec,omitempty"`
	DeferredJSON                string  `json:"tools_deferred_json,omitempty"`
	// ToolWeightJSON stores tool weight analysis results for prompt priority hints.
	ToolWeightJSON string `json:"tool_weight_json,omitempty"`
	// MaxLLMCalls limits the number of LLM calls per turn (0 = unlimited).
	MaxLLMCalls int `json:"max_llm_calls,omitempty"`
	// MaxToolIterations limits the number of tool-call iterations per turn (0 = unlimited).
	MaxToolIterations int `json:"max_tool_iterations,omitempty"`
	// EnableTokenTailoring enables automatic token tailoring for the model's context window.
	EnableTokenTailoring bool `json:"enable_token_tailoring,omitempty"`
	// TokenTailoringStrategy selects the tailoring strategy: "middle_out" (default), "head_out", "tail_out".
	// Maps to framework trpcmodel.TailoringStrategy implementations.
	TokenTailoringStrategy string `json:"token_tailoring_strategy,omitempty"`
	// TokenTailoringSafetyMargin is the safety margin ratio for token counting inaccuracies (0.0–1.0).
	// Maps to framework trpcmodel.TokenTailoringConfig.SafetyMarginRatio.
	TokenTailoringSafetyMargin float64 `json:"token_tailoring_safety_margin,omitempty"`
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

// RalphLoopCfg holds Ralph Loop verification-cycle settings.
type RalphLoopCfg struct {
	MaxIterations        int    `json:"ralph_loop_max_iterations,omitempty"`
	CompletionPromise    string `json:"ralph_loop_completion_promise,omitempty"`
	VerifyCommand        string `json:"ralph_loop_verify_command,omitempty"`
	VerifyTimeoutSeconds int    `json:"ralph_loop_verify_timeout_seconds,omitempty"`
	PromiseTagOpen       string `json:"ralph_loop_promise_tag_open,omitempty"`
	PromiseTagClose      string `json:"ralph_loop_promise_tag_close,omitempty"`
	VerifyWorkDir        string `json:"ralph_loop_verify_work_dir,omitempty"`
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
	SubagentsStoredResultRunes        int     `json:"subagents_stored_result_runes,omitempty"`
	SubagentsStoredSummaryRunes       int     `json:"subagents_stored_summary_runes,omitempty"`
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
	// DreamSnapshotJSON stores dream_cycle execution snapshots for rollback.
	DreamSnapshotJSON string `json:"dream_snapshot_json,omitempty"`
}

// ContextCfg holds context-compaction, output-schema, model-selector, and planner settings.
type ContextCfg struct {
	CompactionEnabled          bool    `json:"context_compaction_enabled,omitempty"`
	MemoryCompactEnabled       bool    `json:"memory_compact_enabled,omitempty"`
	ToolResultGateEnabled      bool    `json:"tool_result_gate_enabled,omitempty"`
	CompressLLMCacheEnabled    bool    `json:"compress_llm_cache_enabled,omitempty"`
	CompressLLMCacheMaxEntries int     `json:"compress_llm_cache_max_entries,omitempty"`
	CompressLLMCacheTTLSec     int     `json:"compress_llm_cache_ttl_sec,omitempty"`
	CompressionBufferRatio     float64 `json:"compression_buffer_ratio,omitempty"`
	CompressionBufferAdaptive  bool    `json:"compression_buffer_adaptive,omitempty"`
	SoftTriggerRatio           float64 `json:"soft_trigger_ratio,omitempty"`
	HardTriggerRatio           float64 `json:"hard_trigger_ratio,omitempty"`
	SessionSummaryEnabled      bool    `json:"session_summary_enabled,omitempty"`
	OutputSchemaJSON           string  `json:"output_schema_json,omitempty"`
	ModelSelector              string  `json:"model_selector,omitempty"`
	PlannerKind                string  `json:"planner_kind,omitempty"`
	PlannerConfigJSON          string  `json:"planner_config_json,omitempty"`
	// VerificationTruncateChars is the max character count for truncating team output in verification gate prompts (default 2000).
	VerificationTruncateChars int `json:"verification_truncate_chars,omitempty"`
	// ClarificationEnabled enables the clarification gate: when the intent pass
	// detects blocking ambiguities, the turn pauses and asks the user for
	// clarification before proceeding. Default true.
	ClarificationEnabled bool `json:"clarification_enabled,omitempty"`
}

func (s *AgentRuntimeSettings) ApplyIdentity(cfg IdentityCfg) {
	s.AgentID = cfg.AgentID
	s.ChannelID = cfg.ChannelID
	s.ChatID = cfg.ChatID
	s.Workspace = cfg.Workspace
	s.VariablesJSON = cfg.VariablesJSON
	s.ModelInstructionsJSON = cfg.ModelInstructionsJSON
}

func (s *AgentRuntimeSettings) ApplyReasoning(cfg ReasoningCfg) {
	s.ReasoningMode = cfg.Mode
	s.ReasoningLevel = cfg.Level
}

func (s *AgentRuntimeSettings) ApplyMemory(cfg MemoryCfg) {
	s.MemoryEnabled = cfg.Enabled
	s.MemoryMaxChunkLength = cfg.MaxChunkLength
	s.MemoryMaxResults = cfg.MaxResults
	s.MemoryMinScore = cfg.MinScore
	s.HeartbeatEnabled = cfg.HeartbeatEnabled
	s.HeartbeatIntervalMinutes = cfg.HeartbeatIntervalMinutes
	s.L0RecentWindowTurns = cfg.L0RecentWindowTurns
	s.L0RecentWindowTokens = cfg.L0RecentWindowTokens
	s.L0SummaryThreshold = cfg.L0SummaryThreshold
	s.L0SummaryKeepTurns = cfg.L0SummaryKeepTurns
	s.L0CompressMinGapSec = cfg.L0CompressMinGapSec
	s.L0CompressProvider = cfg.L0CompressProvider
	s.L0CompressModel = cfg.L0CompressModel
	s.MemoryWorkerProvider = cfg.MemoryWorkerProvider
	s.MemoryWorkerModel = cfg.MemoryWorkerModel
	s.L0TruncateStrategy = cfg.L0TruncateStrategy
	s.L0InjectL1 = cfg.L0InjectL1
	s.L0InjectL3 = cfg.L0InjectL3
	s.L0InjectL4 = cfg.L0InjectL4
	s.L0L3MaxChunks = cfg.L0L3MaxChunks
	s.L0L4MaxPaths = cfg.L0L4MaxPaths
	s.L0SnapshotMode = cfg.L0SnapshotMode
	s.L0SnapshotEnabled = cfg.L0SnapshotEnabled
	s.L1Enabled = cfg.L1Enabled
	s.L1BudgetTokens = cfg.L1BudgetTokens
	s.L1FieldMaxTokens = cfg.L1FieldMaxTokens
	s.L1HistoryKeepRevisions = cfg.L1HistoryKeepRevisions
	s.L1HistoryEnabled = cfg.L1HistoryEnabled
	s.L1DefaultSchemaID = cfg.L1DefaultSchemaID
	s.L1ArchiveOnIdleMinutes = cfg.L1ArchiveOnIdleMinutes
	s.L2EpisodeEnabled = cfg.L2EpisodeEnabled
	s.L2EpisodeMinImportance = cfg.L2EpisodeMinImportance
	s.L2IndexEnabled = cfg.L2IndexEnabled
	s.L2IndexEmbeddingModel = cfg.L2IndexEmbeddingModel
	s.L2RecallEnabled = cfg.L2RecallEnabled
	s.L2RecallMax = cfg.L2RecallMax
	s.L2RetentionDays = cfg.L2RetentionDays
	s.L2ArchiveAfterDays = cfg.L2ArchiveAfterDays
	s.L3Enabled = cfg.L3Enabled
	s.L3RecallTopK = cfg.L3RecallTopK
	s.L3RecallMinScore = cfg.L3RecallMinScore
	s.L3RecallScopesJSON = cfg.L3RecallScopesJSON
	s.L3EmbeddingModel = cfg.L3EmbeddingModel
	s.L3DecayIntervalHours = cfg.L3DecayIntervalHours
	s.L3ArchiveThreshold = cfg.L3ArchiveThreshold
	s.L3MaxPerRecallChars = cfg.L3MaxPerRecallChars
	s.L3RecallBudgetTokens = cfg.L3RecallBudgetTokens
	s.L4Enabled = cfg.L4Enabled
	s.L4GraphInjectNeighbors = cfg.L4GraphInjectNeighbors
	s.L4GraphMaxNeighbors = cfg.L4GraphMaxNeighbors
	s.L4GraphMaxHops = cfg.L4GraphMaxHops
	s.L4IdentityInject = cfg.L4IdentityInject
	s.L4StrategyInject = cfg.L4StrategyInject
	s.L4DecayIntervalHours = cfg.L4DecayIntervalHours
	s.L4DecayOverridesJSON = cfg.L4DecayOverridesJSON
	s.ForgetConfigJSON = cfg.ForgetConfigJSON
}

func (s *AgentRuntimeSettings) ApplyTools(cfg ToolsCfg) {
	s.ToolsEnabled = cfg.Enabled
	s.ToolsProfile = cfg.Profile
	s.ToolsToolCallPrefix = cfg.ToolCallPrefix
	s.ToolsAllowJSON = cfg.AllowJSON
	s.ToolsDenyJSON = cfg.DenyJSON
	s.ToolsConcurrentAllowJSON = cfg.ConcurrentAllowJSON
	s.ToolsRetryEnabled = cfg.RetryEnabled
	s.ToolsRetryMaxAttempts = cfg.RetryMaxAttempts
	s.ToolsRetryInitialIntervalMs = cfg.RetryInitialIntervalMs
	s.ToolsRetryBackoffFactor = cfg.RetryBackoffFactor
	s.ToolsRetryMaxIntervalMs = cfg.RetryMaxIntervalMs
	s.ToolsRetryJitter = cfg.RetryJitter
	s.ToolsParallelEnabled = cfg.ParallelEnabled
	s.ToolsStreamingEnabled = cfg.StreamingEnabled
	s.ToolsCircuitBreakerEnabled = cfg.CircuitBreakerEnabled
	s.ToolsCircuitBreakerOverridesJSON = cfg.CircuitBreakerOverridesJSON
	s.ToolsDeferredJSON = cfg.DeferredJSON
	s.ToolsCommandSafetyEnabled = cfg.CommandSafetyEnabled
	s.ToolsExecutionTimeoutSec = cfg.ExecutionTimeoutSec
	s.ToolWeightJSON = cfg.ToolWeightJSON
	s.MaxLLMCalls = cfg.MaxLLMCalls
	s.MaxToolIterations = cfg.MaxToolIterations
	s.EnableTokenTailoring = cfg.EnableTokenTailoring
	s.TokenTailoringStrategy = cfg.TokenTailoringStrategy
	s.TokenTailoringSafetyMargin = cfg.TokenTailoringSafetyMargin
}

func (s *AgentRuntimeSettings) ApplySkills(cfg SkillsCfg) {
	s.SkillRuntimeJSON = cfg.RuntimeJSON
	s.SkillLoadMode = cfg.LoadMode
	s.IntentPassEnabled = cfg.IntentPassEnabled
}

func (s *AgentRuntimeSettings) GetSkillLoadMode() string {
	if s == nil {
		return ""
	}
	mode := strings.TrimSpace(s.SkillLoadMode)
	if mode == "" {
		return ""
	}
	return mode
}

func (s *AgentRuntimeSettings) ApplyEvolution(cfg EvolutionCfg) {
	s.SelfEvolve = cfg.SelfEvolve
	s.EvolutionSelfEvolve = cfg.SelfEvolve
	s.SubagentsEnabled = cfg.SubagentsEnabled
	s.SubagentsMaxConcurrency = cfg.SubagentsMaxConcurrency
	s.SubagentsMaxGenerationDepth = cfg.SubagentsMaxGenerationDepth
	s.SubagentsMaxChildrenPerAgent = cfg.SubagentsMaxChildrenPerAgent
	s.SubagentsArchiveAfterMinutes = cfg.SubagentsArchiveAfterMinutes
	s.SubagentsMaxRetries = cfg.SubagentsMaxRetries
	s.SubagentsModelOverride = cfg.SubagentsModelOverride
	s.SubagentsStoredResultRunes = cfg.SubagentsStoredResultRunes
	s.SubagentsStoredSummaryRunes = cfg.SubagentsStoredSummaryRunes
	s.EvolutionSkillEvolve = cfg.SkillEvolve
	s.EvolutionMetricsEnabled = cfg.MetricsEnabled
	s.EvolutionSuggestionsEnabled = cfg.SuggestionsEnabled
	s.GuardrailMaxChangePerPeriod = cfg.GuardrailMaxChangePerPeriod
	s.GuardrailMinDataPoints = cfg.GuardrailMinDataPoints
	s.GuardrailRollbackOnDeclinePercent = cfg.GuardrailRollbackOnDeclinePercent
	s.EvoEnabled = cfg.EvoEnabled
	s.EvoAutoApply = cfg.EvoAutoApply
	s.EvoMinEpisodes = cfg.EvoMinEpisodes
	s.EvoMinNegativeFeedback = cfg.EvoMinNegativeFeedback
	s.EvoThrottleHours = cfg.EvoThrottleHours
	s.EvoProposalTTLDays = cfg.EvoProposalTTLDays
	s.EvoPersonaMaxChars = cfg.EvoPersonaMaxChars
	s.EvoSystemPromptMaxAppends = cfg.EvoSystemPromptMaxAppends
	s.DreamSnapshotJSON = cfg.DreamSnapshotJSON
}

func (s *AgentRuntimeSettings) ApplyContext(cfg ContextCfg) {
	s.ContextCompactionEnabled = cfg.CompactionEnabled
	s.MemoryCompactEnabled = cfg.MemoryCompactEnabled
	s.ToolResultGateEnabled = cfg.ToolResultGateEnabled
	s.CompressLLMCacheEnabled = cfg.CompressLLMCacheEnabled
	s.CompressLLMCacheMaxEntries = cfg.CompressLLMCacheMaxEntries
	s.CompressLLMCacheTTLSec = cfg.CompressLLMCacheTTLSec
	s.CompressionBufferRatio = cfg.CompressionBufferRatio
	s.CompressionBufferAdaptive = cfg.CompressionBufferAdaptive
	s.SoftTriggerRatio = cfg.SoftTriggerRatio
	s.HardTriggerRatio = cfg.HardTriggerRatio
	s.SessionSummaryEnabled = cfg.SessionSummaryEnabled
	s.OutputSchemaJSON = cfg.OutputSchemaJSON
	s.ModelSelector = cfg.ModelSelector
	s.PlannerKind = cfg.PlannerKind
	s.PlannerConfigJSON = cfg.PlannerConfigJSON
	s.VerificationTruncateChars = cfg.VerificationTruncateChars
	s.ClarificationEnabled = cfg.ClarificationEnabled
}

func (s *AgentRuntimeSettings) ApplyRalphLoop(cfg RalphLoopCfg) {
	s.RalphLoopMaxIterations = cfg.MaxIterations
	s.RalphLoopCompletionPromise = cfg.CompletionPromise
	s.RalphLoopVerifyCommand = cfg.VerifyCommand
	s.RalphLoopVerifyTimeoutSeconds = cfg.VerifyTimeoutSeconds
	s.RalphLoopPromiseTagOpen = cfg.PromiseTagOpen
	s.RalphLoopPromiseTagClose = cfg.PromiseTagClose
	s.RalphLoopVerifyWorkDir = cfg.VerifyWorkDir
}

// --- Sub-domain read interfaces ---
// Consumers should depend on the smallest interface they need,
// not the entire AgentRuntimeSettings struct.

// IdentityReader provides read access to identity/routing settings.
// Stability:evolving
type IdentityReader interface {
	GetIdentity() IdentityCfg
}

// ReasoningReader provides read access to reasoning strategy settings.
// Stability:evolving
type ReasoningReader interface {
	GetReasoning() ReasoningCfg
}

// MemoryReader provides read access to memory (L0-L4) settings.
// Stability:evolving
type MemoryReader interface {
	GetMemory() MemoryCfg
}

// ToolsReader provides read access to tool configuration settings.
// Stability:evolving
type ToolsReader interface {
	GetTools() ToolsCfg
}

// SkillsReader provides read access to skill and code executor settings.
// Stability:evolving
type SkillsReader interface {
	GetSkills() SkillsCfg
	GetCodeExecutor() CodeExecutorCfg
}

// EvolutionReader provides read access to evolution, ralph loop, and dream settings.
// Stability:evolving
type EvolutionReader interface {
	GetEvolution() EvolutionCfg
	GetRalphLoop() RalphLoopCfg
}

// ContextReader provides read access to context compression and planner settings.
// Stability:evolving
type ContextReader interface {
	GetContext() ContextCfg
}

// RuntimeSettingsReader is the aggregate read interface combining all sub-domains.
// Use this when you need access to multiple sub-domains.
// Stability:evolving
type RuntimeSettingsReader interface {
	IdentityReader
	ReasoningReader
	MemoryReader
	ToolsReader
	SkillsReader
	EvolutionReader
	ContextReader
}

// RuntimeSettingsWriter is the aggregate write interface for all sub-domains.
// Stability:evolving
type RuntimeSettingsWriter interface {
	ApplyIdentity(IdentityCfg)
	ApplyReasoning(ReasoningCfg)
	ApplyMemory(MemoryCfg)
	ApplyTools(ToolsCfg)
	ApplySkills(SkillsCfg)
	ApplyEvolution(EvolutionCfg)
	ApplyRalphLoop(RalphLoopCfg)
	ApplyContext(ContextCfg)
}
