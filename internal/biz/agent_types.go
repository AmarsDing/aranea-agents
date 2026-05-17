package biz

// Agent is the catalog agent aggregate (legacy agents table + hydrated runtime state).
type Agent struct {
	ID                 string
	AgentKey           string
	DisplayName        string
	Provider           string
	Model              string
	Status             string
	IsDefault          bool
	IsFavorite         bool
	Icon               string
	AgentDescription   string
	CategoryPositionID string
	SystemPromptMode   string
	ContextWindow      int
	BudgetMonthlyCents int
	ConfigJSON         string
	Roles              []string
	CreatedAt          string
	UpdatedAt          string
	DeletedAt          string
	Settings           *AgentRuntimeSettings
	Files              []AgentPromptFile
}

// AgentRuntimeSettings mirrors the agent_runtime_settings row.
// Fields are kept flat for Ent/DB compatibility; domain accessors are provided
// via the methods below (see agent_settings.go for domain sub-struct types).
//
// Domains:
//   - Identity  : AgentID, ChannelID, ChatID, Workspace, VariablesJSON, ModelInstructionsJSON
//   - Reasoning : ReasoningMode, ReasoningLevel
//   - Memory    : MemoryEnabled, MemoryMax*, HeartbeatEnabled, L0–L4 fields
//   - Tools     : ToolsEnabled, ToolsProfile, ToolsToolCallPrefix, ToolsAllow/Deny, ToolsRetry*, ToolsParallel*, ToolsStreaming*
//   - Skills    : SkillRuntimeJSON, IntentPassEnabled, SkillLoadMode
//   - Plugins   : (reserved)
//   - Evolution : SelfEvolve, Subagents*, Evolution*, Guardrail*, Evo*
//   - Context   : ContextCompactionEnabled, SessionSummaryEnabled, OutputSchemaJSON, ModelSelector, PlannerKind
type AgentRuntimeSettings struct {
	AgentID                           string
	SelfEvolve                        bool
	SubagentsEnabled                  bool
	SubagentsMaxConcurrency           int
	SubagentsMaxGenerationDepth       int
	SubagentsMaxChildrenPerAgent      int
	SubagentsArchiveAfterMinutes      int
	SubagentsMaxRetries               int
	SubagentsModelOverride            string
	ToolsEnabled                      bool
	ToolsProfile                      string
	ToolsToolCallPrefix               string
	ToolsAllowJSON                    string
	ToolsDenyJSON                     string
	ToolsConcurrentAllowJSON          string
	MemoryEnabled                     bool
	MemoryMaxChunkLength              int
	MemoryMaxResults                  int
	MemoryMinScore                    float64
	HeartbeatEnabled                  bool
	HeartbeatIntervalMinutes          int
	EvolutionSelfEvolve               bool
	EvolutionSkillEvolve              bool
	EvolutionMetricsEnabled           bool
	EvolutionSuggestionsEnabled       bool
	GuardrailMaxChangePerPeriod       float64
	GuardrailMinDataPoints            int
	GuardrailRollbackOnDeclinePercent int
	L0RecentWindowTurns               int
	L0RecentWindowTokens              int
	L0SummaryThreshold                float64
	L0SummaryKeepTurns                int
	// L0CompressProvider / L0CompressModel select a cheaper catalog model for session summarization; empty → use agent/session chat model.
	L0CompressProvider        string
	L0CompressModel           string
	L0TruncateStrategy        string
	L0InjectL1                bool
	L0InjectL3                bool
	L0InjectL4                bool
	L0L3MaxChunks             int
	L0L4MaxPaths              int
	L0SnapshotMode            string
	L1Enabled                 bool
	L1BudgetTokens            int
	L1FieldMaxTokens          int
	L1HistoryKeepRevisions    int
	L1DefaultSchemaID         string
	L1ArchiveOnIdleMinutes    int
	L2EpisodeEnabled          bool
	L2EpisodeMinImportance    float64
	L2IndexEnabled            bool
	L2IndexEmbeddingModel     string
	L2RecallEnabled           bool
	L2RecallMax               int
	L2RetentionDays           int
	L2ArchiveAfterDays        int
	L3Enabled                 bool
	L3RecallTopK              int
	L3RecallMinScore          float64
	L3RecallScopesJSON        string
	L3EmbeddingModel          string
	L3DecayIntervalHours      int
	L3ArchiveThreshold        float64
	L3MaxPerRecallChars       int
	L4Enabled                 bool
	L4GraphInjectNeighbors    bool
	L4GraphMaxNeighbors       int
	L4GraphMaxHops            int
	L4IdentityInject          bool
	L4StrategyInject          bool
	EvoEnabled                bool
	EvoAutoApply              bool
	EvoMinEpisodes            int
	EvoMinNegativeFeedback    int
	EvoThrottleHours          int
	EvoProposalTTLDays        int
	EvoPersonaMaxChars        int
	EvoSystemPromptMaxAppends int
	// SkillRuntimeJSON is agent_runtime_settings.skill_runtime_json (whitelist/deny/tags + routing caps).
	SkillRuntimeJSON string
	// IntentPassEnabled runs the optional pre-turn intent classification pass (see intent package); default true in DB/UI.
	IntentPassEnabled bool
	ChannelID         string
	ChatID            string
	Workspace         string
	ReasoningMode     string
	ReasoningLevel    string
	VariablesJSON     string
	// ModelInstructionsJSON holds per-model instruction overrides: {"gpt-4o": "...", "claude-3": "..."}.
	ModelInstructionsJSON string
	// ContextCompactionEnabled enables automatic context compaction when tokens approach the limit.
	ContextCompactionEnabled bool
	// SessionSummaryEnabled enables session summary injection so new sessions can inherit old context.
	SessionSummaryEnabled bool
	// SkillLoadMode controls skill loading strategy: "auto" | "manual" | "none".
	SkillLoadMode string
	// OutputSchemaJSON is a JSON Schema that forces the LLM to produce structured output.
	OutputSchemaJSON string
	// ModelSelector controls dynamic model selection: "default" | "auto".
	ModelSelector string
	// ToolsRetryEnabled enables automatic retry for transient tool call failures.
	ToolsRetryEnabled bool
	// ToolsRetryMaxAttempts is the total number of attempts including the first try.
	ToolsRetryMaxAttempts int
	// ToolsRetryInitialIntervalMs is the delay in ms before the second attempt.
	ToolsRetryInitialIntervalMs int
	// ToolsRetryBackoffFactor controls how the delay grows after each failed attempt.
	ToolsRetryBackoffFactor float64
	// ToolsRetryMaxIntervalMs caps the computed retry delay.
	ToolsRetryMaxIntervalMs int
	// ToolsRetryJitter enables additive random jitter on the computed delay.
	ToolsRetryJitter bool
	// ToolsParallelEnabled enables parallel tool execution when the model issues multiple tool calls.
	ToolsParallelEnabled bool
	// ToolsStreamingEnabled enables StreamableTool support for tools that implement StreamableCall.
	ToolsStreamingEnabled bool
	// PlannerKind selects the planning strategy: "" | "builtin" | "react" | "a2ui".
	// Empty string inherits the legacy dialog-mode based selection (builtin when dialogMode="plan").
	PlannerKind string
	CreatedAt   string
	UpdatedAt   string
}

// --- Domain view accessors (Q-22: sub-struct API, flat fields as source of truth) ---

// GetIdentity returns the Identity domain view.
func (s *AgentRuntimeSettings) GetIdentity() IdentityCfg {
	return IdentityCfg{
		AgentID:               s.AgentID,
		ChannelID:             s.ChannelID,
		ChatID:                s.ChatID,
		Workspace:             s.Workspace,
		VariablesJSON:         s.VariablesJSON,
		ModelInstructionsJSON: s.ModelInstructionsJSON,
	}
}

// GetReasoning returns the Reasoning domain view.
func (s *AgentRuntimeSettings) GetReasoning() ReasoningCfg {
	return ReasoningCfg{Mode: s.ReasoningMode, Level: s.ReasoningLevel}
}

// GetMemory returns the Memory domain view (L0–L4).
func (s *AgentRuntimeSettings) GetMemory() MemoryCfg {
	return MemoryCfg{
		Enabled:                  s.MemoryEnabled,
		MaxChunkLength:           s.MemoryMaxChunkLength,
		MaxResults:               s.MemoryMaxResults,
		MinScore:                 s.MemoryMinScore,
		HeartbeatEnabled:         s.HeartbeatEnabled,
		HeartbeatIntervalMinutes: s.HeartbeatIntervalMinutes,
		L0RecentWindowTurns:      s.L0RecentWindowTurns,
		L0RecentWindowTokens:     s.L0RecentWindowTokens,
		L0SummaryThreshold:       s.L0SummaryThreshold,
		L0SummaryKeepTurns:       s.L0SummaryKeepTurns,
		L0CompressProvider:       s.L0CompressProvider,
		L0CompressModel:          s.L0CompressModel,
		L0TruncateStrategy:       s.L0TruncateStrategy,
		L0InjectL1:               s.L0InjectL1,
		L0InjectL3:               s.L0InjectL3,
		L0InjectL4:               s.L0InjectL4,
		L0L3MaxChunks:            s.L0L3MaxChunks,
		L0L4MaxPaths:             s.L0L4MaxPaths,
		L0SnapshotMode:           s.L0SnapshotMode,
		L1Enabled:                s.L1Enabled,
		L1BudgetTokens:           s.L1BudgetTokens,
		L1FieldMaxTokens:         s.L1FieldMaxTokens,
		L1HistoryKeepRevisions:   s.L1HistoryKeepRevisions,
		L1DefaultSchemaID:        s.L1DefaultSchemaID,
		L1ArchiveOnIdleMinutes:   s.L1ArchiveOnIdleMinutes,
		L2EpisodeEnabled:         s.L2EpisodeEnabled,
		L2EpisodeMinImportance:   s.L2EpisodeMinImportance,
		L2IndexEnabled:           s.L2IndexEnabled,
		L2IndexEmbeddingModel:    s.L2IndexEmbeddingModel,
		L2RecallEnabled:          s.L2RecallEnabled,
		L2RecallMax:              s.L2RecallMax,
		L2RetentionDays:          s.L2RetentionDays,
		L2ArchiveAfterDays:       s.L2ArchiveAfterDays,
		L3Enabled:                s.L3Enabled,
		L3RecallTopK:             s.L3RecallTopK,
		L3RecallMinScore:         s.L3RecallMinScore,
		L3RecallScopesJSON:       s.L3RecallScopesJSON,
		L3EmbeddingModel:         s.L3EmbeddingModel,
		L3DecayIntervalHours:     s.L3DecayIntervalHours,
		L3ArchiveThreshold:       s.L3ArchiveThreshold,
		L3MaxPerRecallChars:      s.L3MaxPerRecallChars,
		L4Enabled:                s.L4Enabled,
		L4GraphInjectNeighbors:   s.L4GraphInjectNeighbors,
		L4GraphMaxNeighbors:      s.L4GraphMaxNeighbors,
		L4GraphMaxHops:           s.L4GraphMaxHops,
		L4IdentityInject:         s.L4IdentityInject,
		L4StrategyInject:         s.L4StrategyInject,
	}
}

// GetTools returns the Tools domain view.
func (s *AgentRuntimeSettings) GetTools() ToolsCfg {
	return ToolsCfg{
		Enabled:                s.ToolsEnabled,
		Profile:                s.ToolsProfile,
		ToolCallPrefix:         s.ToolsToolCallPrefix,
		AllowJSON:              s.ToolsAllowJSON,
		DenyJSON:               s.ToolsDenyJSON,
		ConcurrentAllowJSON:    s.ToolsConcurrentAllowJSON,
		RetryEnabled:           s.ToolsRetryEnabled,
		RetryMaxAttempts:       s.ToolsRetryMaxAttempts,
		RetryInitialIntervalMs: s.ToolsRetryInitialIntervalMs,
		RetryBackoffFactor:     s.ToolsRetryBackoffFactor,
		RetryMaxIntervalMs:     s.ToolsRetryMaxIntervalMs,
		RetryJitter:            s.ToolsRetryJitter,
		ParallelEnabled:        s.ToolsParallelEnabled,
		StreamingEnabled:       s.ToolsStreamingEnabled,
	}
}

// GetSkills returns the Skills domain view.
func (s *AgentRuntimeSettings) GetSkills() SkillsCfg {
	return SkillsCfg{
		RuntimeJSON:       s.SkillRuntimeJSON,
		LoadMode:          s.SkillLoadMode,
		IntentPassEnabled: s.IntentPassEnabled,
	}
}

// GetEvolution returns the Evolution domain view.
func (s *AgentRuntimeSettings) GetEvolution() EvolutionCfg {
	return EvolutionCfg{
		SelfEvolve:                        s.SelfEvolve,
		SubagentsEnabled:                  s.SubagentsEnabled,
		SubagentsMaxConcurrency:           s.SubagentsMaxConcurrency,
		SubagentsMaxGenerationDepth:       s.SubagentsMaxGenerationDepth,
		SubagentsMaxChildrenPerAgent:      s.SubagentsMaxChildrenPerAgent,
		SubagentsArchiveAfterMinutes:      s.SubagentsArchiveAfterMinutes,
		SubagentsMaxRetries:               s.SubagentsMaxRetries,
		SubagentsModelOverride:            s.SubagentsModelOverride,
		SkillEvolve:                       s.EvolutionSkillEvolve,
		MetricsEnabled:                    s.EvolutionMetricsEnabled,
		SuggestionsEnabled:                s.EvolutionSuggestionsEnabled,
		GuardrailMaxChangePerPeriod:       s.GuardrailMaxChangePerPeriod,
		GuardrailMinDataPoints:            s.GuardrailMinDataPoints,
		GuardrailRollbackOnDeclinePercent: s.GuardrailRollbackOnDeclinePercent,
		EvoEnabled:                        s.EvoEnabled,
		EvoAutoApply:                      s.EvoAutoApply,
		EvoMinEpisodes:                    s.EvoMinEpisodes,
		EvoMinNegativeFeedback:            s.EvoMinNegativeFeedback,
		EvoThrottleHours:                  s.EvoThrottleHours,
		EvoProposalTTLDays:                s.EvoProposalTTLDays,
		EvoPersonaMaxChars:                s.EvoPersonaMaxChars,
		EvoSystemPromptMaxAppends:         s.EvoSystemPromptMaxAppends,
	}
}

// GetContext returns the Context domain view.
func (s *AgentRuntimeSettings) GetContext() ContextCfg {
	return ContextCfg{
		CompactionEnabled:     s.ContextCompactionEnabled,
		SessionSummaryEnabled: s.SessionSummaryEnabled,
		OutputSchemaJSON:      s.OutputSchemaJSON,
		ModelSelector:         s.ModelSelector,
		PlannerKind:           s.PlannerKind,
	}
}

// AgentPromptFile is one row in agent_prompt_files (API name field maps to file_name).
type AgentPromptFile struct {
	ID        string
	AgentID   string
	Name      string
	Body      string
	SortOrder int
	CreatedAt string
	UpdatedAt string
}

// AgentListQuery filters the agent catalog list.
type AgentListQuery struct {
	Keyword    string
	Status     string
	Provider   string
	CategoryID string
	Limit      int
	Offset     int
}

// AgentListResult is a page of agents without per-row hydration unless noted.
type AgentListResult struct {
	Items  []Agent
	Total  int
	Limit  int
	Offset int
}

// FileTokenEstimate is the token estimate for a single prompt file.
type FileTokenEstimate struct {
	FileID          string
	FileName        string
	EstimatedTokens int
}

// FileTokenEstimates is the aggregate token estimate for all prompt files of an agent.
type FileTokenEstimates struct {
	TotalTokens   int
	FileEstimates []FileTokenEstimate
}
