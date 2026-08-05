package biz

import (
	"encoding/json"
	"strings"

	"aranea-agents/pkg/apierror"
)

const (
	SpiritAgentKey      = "__spirit__"
	MemoryAgentKey      = "__memory__"
	SkillsAgentKey      = "__skills__"
	SystemAdminAgentKey = "__system_admin__"

	// Default compression trigger ratios (single source of truth).
	DefaultCompressionBufferRatio = 0.15
	DefaultSoftTriggerRatio       = 0.70
	DefaultHardTriggerRatio       = 0.90

	// DefaultToolsDenyFrameworkMemory lists the framework memory tools that are denied
	// by default for new agents (working_memory mode). Agents using "both" mode
	// should clear these from their ToolsDenyJSON.
	DefaultToolsDenyFrameworkMemory = `["memory_add","memory_update","memory_delete","memory_search","memory_load"]`
)

// IsSystemAgentKey reports whether the given key is a built-in system agent
// that should never participate in business task teams. System agents are
// infrastructure-level (spirit orchestrator, memory/skills/system admin) and
// must not be selected as team members by the allocator.
//
// 2026-07-04 问题 3 修复：统一系统 Agent 判断逻辑，避免每个过滤点重复写常量。
func IsSystemAgentKey(key string) bool {
	switch key {
	case SpiritAgentKey, SystemAdminAgentKey, MemoryAgentKey, SkillsAgentKey:
		return true
	}
	return false
}

// AgentStatus enumerates the valid lifecycle statuses for a catalog agent.
// Using typed constants instead of free-form strings prevents invalid status values.
type AgentStatus string

const (
	AgentStatusActive   AgentStatus = "active"
	AgentStatusInactive AgentStatus = "inactive"
	AgentStatusArchived AgentStatus = "archived"
)

// ValidAgentStatuses is the set of all valid agent statuses.
var ValidAgentStatuses = map[AgentStatus]bool{
	AgentStatusActive:   true,
	AgentStatusInactive: true,
	AgentStatusArchived: true,
}

// ValidateAgentStatus returns an error if the given status string is not a valid AgentStatus.
func ValidateAgentStatus(status string) error {
	if status == "" {
		return nil // empty is allowed (defaults to active on creation)
	}
	if ValidAgentStatuses[AgentStatus(status)] {
		return nil
	}
	return apierror.BadRequest("AGENT", "invalid agent status: "+status)
}

// NormalizeAgentStatus returns the normalized AgentStatus, defaulting to active for empty.
func NormalizeAgentStatus(status string) AgentStatus {
	s := AgentStatus(strings.TrimSpace(strings.ToLower(status)))
	if s == "" {
		return AgentStatusActive
	}
	if ValidAgentStatuses[s] {
		return s
	}
	return AgentStatusActive
}

// agentEventForTarget determines the AgentEvent needed to transition to the given target state.
// This is used by AgentUsecase to validate status transitions via the state machine.
func agentEventForTarget(target AgentState) AgentEvent {
	switch target {
	case AgentStateActive:
		return AgentEventActivate
	case AgentStateInactive:
		return AgentEventDeactivate
	case AgentStateArchived:
		return AgentEventArchive
	default:
		return AgentEvent(target) // fallback; will fail transition validation
	}
}

// BoolVal dereferences a *bool, returning false for nil.
func BoolVal(p *bool) bool { return p != nil && *p }

// BoolEqual reports whether two *bool values are semantically equal.
// nil and false are treated as equivalent (both mean "not set / false").
func BoolEqual(a, b *bool) bool {
	return BoolVal(a) == BoolVal(b)
}

type Agent struct {
	ID                    string
	AgentKey              string
	DisplayName           string
	Provider              string
	Model                 string
	Status                string
	IsDefault             *bool // nil = not set (Proto3 zero-value ambiguity); explicit true/false for merge
	IsFavorite            *bool // nil = not set (Proto3 zero-value ambiguity); explicit true/false for merge
	Icon                  string
	AgentDescription      string
	PositionID            string
	PositionKey           string
	AgentVariant          string
	VariantDescription    string
	SystemPromptMode      string
	ContextWindow         int
	BudgetMonthlyCents    int
	ConfigJSON            string
	MetadataJSON          string
	Roles                 []string
	Kind                  string // user | system_builtin | ecosystem_preset | marketplace | certified (ownership, maps from DB kind column)
	AgentKind             string // llm | a2a_proxy (technical type, derived from config_json by HydrateAgentKind)
	A2AProxy              *A2AProxyConfig
	A2AEndpointEnabled    bool   // list/get enrichment from a2a_agent_cards.enabled
	LastRunStatus         string // list enrichment: latest session runtime.status or idle/completed
	LastRunAt             string
	PendingEvolutionCount int
	CreatedBy             string
	Readonly              bool
	Source                string // user | system | imported (origin, maps from DB source column)
	CreatedAt             string
	UpdatedAt             string
	DeletedAt             string
	Settings              *AgentRuntimeSettings
	Files                 []AgentPromptFile
	// CategoryResponsibilityPreview is a transient field populated by the prompt
	// preview handler to display the injected role_responsibility block.
	// It is never persisted to DB. PGO-1-BIZ-03.
	CategoryResponsibilityPreview string
	// WorkspaceID is the owning workspace ID for tenant isolation (P2-B).
	// empty = shared/legacy (visible to all workspaces); non-empty = tenant-private.
	WorkspaceID string
	// MissionStatement 是 Agent 的长期使命（出生登记于 AgentFactory，手工 Agent 可空）。
	// 匹配时为空回退 AgentDescription（不变量 2）。
	MissionStatement string
	// DomainPath 是归一化领域路径（如 "创作/文学"），空 = 未分类（走旧匹配管线）。
	DomainPath string
}

// SkipCategoryResponsibility returns true when the agent's metadata_json
// contains {"skip_category_responsibility": true}, allowing power users to
// opt out of the automatic L1 岗位职责 injection. PGO-1-BIZ-05.
func (a Agent) SkipCategoryResponsibility() bool {
	if strings.TrimSpace(a.MetadataJSON) == "" {
		return false
	}
	var m struct {
		Skip bool `json:"skip_category_responsibility"`
	}
	if err := json.Unmarshal([]byte(a.MetadataJSON), &m); err != nil {
		return false
	}
	return m.Skip
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
//   - Skills    : SkillRuntimeJSON, IntentPassEnabled, SkillLoadMode, CodeExecutorType
//   - Plugins   : (reserved)
//   - Evolution : SelfEvolve, Subagents*, Evolution*, Guardrail*, Evo*
//   - Context   : ContextCompactionEnabled, SessionSummaryEnabled, OutputSchemaJSON, ModelSelector, PlannerKind
type AgentRuntimeSettings struct {
	AgentID string
	// Deprecated: use EvolutionSelfEvolve
	SelfEvolve                        bool
	SubagentsEnabled                  bool
	SubagentsMaxConcurrency           int
	SubagentsMaxGenerationDepth       int
	SubagentsMaxChildrenPerAgent      int
	SubagentsArchiveAfterMinutes      int
	SubagentsMaxRetries               int
	SubagentsModelOverride            string
	SubagentsStoredResultRunes        int
	SubagentsStoredSummaryRunes       int
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
	// L0CompressMinGapSec is the minimum seconds between automatic session compressions (0 → default 600).
	L0CompressMinGapSec int
	// L0CompressProvider / L0CompressModel select a cheaper catalog model for session summarization; empty → use agent/session chat model.
	L0CompressProvider string
	L0CompressModel    string
	// MemoryWorkerProvider / MemoryWorkerModel select the LLM for async memory extraction; empty → L0 compress, then chat model.
	MemoryWorkerProvider      string
	MemoryWorkerModel         string
	L0TruncateStrategy        string
	L0InjectL1                bool
	L0InjectL3                bool
	L0InjectL4                bool
	L0L3MaxChunks             int
	L0L4MaxPaths              int
	L0SnapshotMode            string
	L0SnapshotEnabled         bool
	L1Enabled                 bool
	L1BudgetTokens            int
	L1FieldMaxTokens          int
	L1HistoryKeepRevisions    int
	L1DefaultSchemaID         string
	L1HistoryEnabled          bool
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
	L3RecallBudgetTokens      int
	L4Enabled                 bool
	L4GraphInjectNeighbors    bool
	L4GraphMaxNeighbors       int
	L4GraphMaxHops            int
	L4IdentityInject          bool
	L4StrategyInject          bool
	L4DecayIntervalHours      int
	L4DecayOverridesJSON      string
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
	// IntentPassEnabled runs the optional pre-turn intent classification pass (see intent package); default true for new agents (P1-1).
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
	// MemoryCompactEnabled enables MemoryCompact (reuse extracted memory facts) as the second compression tier.
	MemoryCompactEnabled       bool
	ToolResultGateEnabled      bool
	CompressLLMCacheEnabled    bool
	CompressLLMCacheMaxEntries int
	CompressLLMCacheTTLSec     int
	// CompressionBufferRatio is the fraction of contextWindow reserved as compression buffer (default 0.15, range 0.10–0.25).
	CompressionBufferRatio float64
	// CompressionBufferAdaptive enables adaptive buffer ratio adjustment based on token increment patterns (default true).
	CompressionBufferAdaptive bool
	// SoftTriggerRatio is the fraction of effective_budget at which async compression triggers (default 0.70).
	SoftTriggerRatio float64
	// HardTriggerRatio is the fraction of effective_budget at which sync compression triggers (default 0.90).
	HardTriggerRatio float64
	// SessionSummaryEnabled enables session summary injection so new sessions can inherit old context.
	SessionSummaryEnabled bool
	// SkillLoadMode controls skill loading strategy: "auto" | "manual" | "none".
	SkillLoadMode string
	// CodeExecutorType selects the Skill code execution backend: local | docker | e2b | container.
	CodeExecutorType string
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
	ToolsStreamingEnabled            bool
	ToolsCircuitBreakerEnabled       bool
	ToolsCircuitBreakerOverridesJSON string
	ToolsDeferredJSON                string
	ToolsCommandSafetyEnabled        bool
	// ToolsExecutionTimeoutSec is the per-tool execution timeout in seconds (0 = use default).
	// This covers BeforeTool callbacks, actual execution, and AfterTool callbacks.
	// A safety-net default is applied in the agent layer when this is zero.
	ToolsExecutionTimeoutSec int
	// MaxLLMCalls limits the number of LLM calls per turn (0 = unlimited).
	// Maps to framework llmagent.WithMaxLLMCalls.
	MaxLLMCalls int
	// MaxToolIterations limits the number of tool-call iterations per turn (0 = unlimited).
	// Maps to framework llmagent.WithMaxToolIterations.
	MaxToolIterations int
	// EnableTokenTailoring enables automatic token tailoring for the model's context window.
	EnableTokenTailoring bool
	// TokenTailoringStrategy selects the tailoring strategy: "middle_out" (default), "head_out", "tail_out".
	// Maps to framework trpcmodel.TailoringStrategy implementations.
	TokenTailoringStrategy string
	// TokenTailoringSafetyMargin is the safety margin ratio for token counting inaccuracies (0.0–1.0).
	// Maps to framework trpcmodel.TokenTailoringConfig.SafetyMarginRatio.
	TokenTailoringSafetyMargin float64
	// PlannerKind selects the planning strategy: "" | "builtin" | "react" | "a2ui".
	// Empty string inherits the legacy dialog-mode based selection (builtin when dialogMode="plan").
	PlannerKind string
	// PlannerConfigJSON holds planner-specific options; shape depends on PlannerKind.
	PlannerConfigJSON string
	// Ralph Loop: enabled when any of MaxIterations / CompletionPromise / VerifyCommand is set;
	// persistence requires CompletionPromise and/or VerifyCommand (see ValidateRalphLoopSettings).
	RalphLoopMaxIterations        int
	RalphLoopCompletionPromise    string
	RalphLoopVerifyCommand        string
	RalphLoopVerifyTimeoutSeconds int
	RalphLoopPromiseTagOpen       string
	RalphLoopPromiseTagClose      string
	RalphLoopVerifyWorkDir        string
	// ForgetPolicyJSON stores the memory butler's forget policy configuration.
	ForgetConfigJSON string `json:"forget_policy_json,omitempty"`
	// ToolWeightJSON stores tool weight analysis results for prompt priority hints.
	ToolWeightJSON string `json:"tool_weight_json,omitempty"`
	// DreamSnapshotJSON stores dream_cycle execution snapshots for rollback.
	DreamSnapshotJSON string `json:"dream_snapshot_json,omitempty"`
	// VerificationTruncateChars is the max character count for truncating team output in verification gate prompts (default 2000).
	VerificationTruncateChars int
	// ClarificationEnabled enables the clarification gate: when the intent pass
	// detects blocking ambiguities, the turn pauses and asks the user for
	// clarification before proceeding. Default true for new agents.
	ClarificationEnabled bool
	CreatedAt            string
	UpdatedAt            string
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
		L0CompressMinGapSec:      s.L0CompressMinGapSec,
		L0CompressProvider:       s.L0CompressProvider,
		L0CompressModel:          s.L0CompressModel,
		MemoryWorkerProvider:     s.MemoryWorkerProvider,
		MemoryWorkerModel:        s.MemoryWorkerModel,
		L0TruncateStrategy:       s.L0TruncateStrategy,
		L0InjectL1:               s.L0InjectL1,
		L0InjectL3:               s.L0InjectL3,
		L0InjectL4:               s.L0InjectL4,
		L0L3MaxChunks:            s.L0L3MaxChunks,
		L0L4MaxPaths:             s.L0L4MaxPaths,
		L0SnapshotMode:           s.L0SnapshotMode,
		L0SnapshotEnabled:        s.L0SnapshotEnabled,
		L1Enabled:                s.L1Enabled,
		L1BudgetTokens:           s.L1BudgetTokens,
		L1FieldMaxTokens:         s.L1FieldMaxTokens,
		L1HistoryKeepRevisions:   s.L1HistoryKeepRevisions,
		L1HistoryEnabled:         s.L1HistoryEnabled,
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
		L3RecallBudgetTokens:     s.L3RecallBudgetTokens,
		L4Enabled:                s.L4Enabled,
		L4GraphInjectNeighbors:   s.L4GraphInjectNeighbors,
		L4GraphMaxNeighbors:      s.L4GraphMaxNeighbors,
		L4GraphMaxHops:           s.L4GraphMaxHops,
		L4IdentityInject:         s.L4IdentityInject,
		L4StrategyInject:         s.L4StrategyInject,
		L4DecayIntervalHours:     s.L4DecayIntervalHours,
		L4DecayOverridesJSON:     s.L4DecayOverridesJSON,
		ForgetConfigJSON:         s.ForgetConfigJSON,
	}
}

// GetTools returns the Tools domain view.
func (s *AgentRuntimeSettings) GetTools() ToolsCfg {
	return ToolsCfg{
		Enabled:                     s.ToolsEnabled,
		Profile:                     s.ToolsProfile,
		ToolCallPrefix:              s.ToolsToolCallPrefix,
		AllowJSON:                   s.ToolsAllowJSON,
		DenyJSON:                    s.ToolsDenyJSON,
		ConcurrentAllowJSON:         s.ToolsConcurrentAllowJSON,
		RetryEnabled:                s.ToolsRetryEnabled,
		RetryMaxAttempts:            s.ToolsRetryMaxAttempts,
		RetryInitialIntervalMs:      s.ToolsRetryInitialIntervalMs,
		RetryBackoffFactor:          s.ToolsRetryBackoffFactor,
		RetryMaxIntervalMs:          s.ToolsRetryMaxIntervalMs,
		RetryJitter:                 s.ToolsRetryJitter,
		ParallelEnabled:             s.ToolsParallelEnabled,
		StreamingEnabled:            s.ToolsStreamingEnabled,
		CircuitBreakerEnabled:       s.ToolsCircuitBreakerEnabled,
		CircuitBreakerOverridesJSON: s.ToolsCircuitBreakerOverridesJSON,
		DeferredJSON:                s.ToolsDeferredJSON,
		CommandSafetyEnabled:        s.ToolsCommandSafetyEnabled,
		ExecutionTimeoutSec:         s.ToolsExecutionTimeoutSec,
		ToolWeightJSON:              s.ToolWeightJSON,
		MaxLLMCalls:                 s.MaxLLMCalls,
		MaxToolIterations:           s.MaxToolIterations,
		EnableTokenTailoring:        s.EnableTokenTailoring,
		TokenTailoringStrategy:      s.TokenTailoringStrategy,
		TokenTailoringSafetyMargin:  s.TokenTailoringSafetyMargin,
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

func (s *AgentRuntimeSettings) GetSkillRuntimeJSON() string {
	return s.SkillRuntimeJSON
}

// GetCodeExecutor returns the code executor domain view.
func (s *AgentRuntimeSettings) GetCodeExecutor() CodeExecutorCfg {
	if s == nil {
		return CodeExecutorCfg{Type: "local"}
	}
	t := strings.TrimSpace(s.CodeExecutorType)
	if t == "" {
		t = "local"
	}
	return CodeExecutorCfg{Type: t}
}

// GetEvolution returns the Evolution domain view.
func (s *AgentRuntimeSettings) GetEvolution() EvolutionCfg {
	return EvolutionCfg{
		SelfEvolve:                        s.EvolutionSelfEvolve,
		SubagentsEnabled:                  s.SubagentsEnabled,
		SubagentsMaxConcurrency:           s.SubagentsMaxConcurrency,
		SubagentsMaxGenerationDepth:       s.SubagentsMaxGenerationDepth,
		SubagentsMaxChildrenPerAgent:      s.SubagentsMaxChildrenPerAgent,
		SubagentsArchiveAfterMinutes:      s.SubagentsArchiveAfterMinutes,
		SubagentsMaxRetries:               s.SubagentsMaxRetries,
		SubagentsModelOverride:            s.SubagentsModelOverride,
		SubagentsStoredResultRunes:        s.SubagentsStoredResultRunes,
		SubagentsStoredSummaryRunes:       s.SubagentsStoredSummaryRunes,
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
		DreamSnapshotJSON:                 s.DreamSnapshotJSON,
	}
}

// GetContext returns the Context domain view.
func (s *AgentRuntimeSettings) GetContext() ContextCfg {
	return ContextCfg{
		CompactionEnabled:          s.ContextCompactionEnabled,
		MemoryCompactEnabled:       s.MemoryCompactEnabled,
		ToolResultGateEnabled:      s.ToolResultGateEnabled,
		CompressLLMCacheEnabled:    s.CompressLLMCacheEnabled,
		CompressLLMCacheMaxEntries: s.CompressLLMCacheMaxEntries,
		CompressLLMCacheTTLSec:     s.CompressLLMCacheTTLSec,
		CompressionBufferRatio:     s.CompressionBufferRatio,
		CompressionBufferAdaptive:  s.CompressionBufferAdaptive,
		SoftTriggerRatio:           s.SoftTriggerRatio,
		HardTriggerRatio:           s.HardTriggerRatio,
		SessionSummaryEnabled:      s.SessionSummaryEnabled,
		OutputSchemaJSON:           s.OutputSchemaJSON,
		ModelSelector:              s.ModelSelector,
		PlannerKind:                s.PlannerKind,
		PlannerConfigJSON:          s.PlannerConfigJSON,
		VerificationTruncateChars:  s.VerificationTruncateChars,
		ClarificationEnabled:       s.ClarificationEnabled,
	}
}

// GetRalphLoop returns the RalphLoop domain view.
func (s *AgentRuntimeSettings) GetRalphLoop() RalphLoopCfg {
	return RalphLoopCfg{
		MaxIterations:        s.RalphLoopMaxIterations,
		CompletionPromise:    s.RalphLoopCompletionPromise,
		VerifyCommand:        s.RalphLoopVerifyCommand,
		VerifyTimeoutSeconds: s.RalphLoopVerifyTimeoutSeconds,
		PromiseTagOpen:       s.RalphLoopPromiseTagOpen,
		PromiseTagClose:      s.RalphLoopPromiseTagClose,
		VerifyWorkDir:        s.RalphLoopVerifyWorkDir,
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
	Keyword   string
	Status    string
	Provider  string
	OrgNodeID string
	CreatedBy string
	Role      string
	Kind      string // filter by ownership classification (user | system_builtin | ecosystem_preset | marketplace | certified)
	// WorkspaceID filters by tenant visibility (P2-B):
	// empty = system caller (see all); non-empty = tenant caller (see shared + own).
	WorkspaceID string
	Limit       int
	Offset      int
}

// AgentCreator is a distinct agent creator for list filters.
type AgentCreator struct {
	UserID string
	Label  string
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
