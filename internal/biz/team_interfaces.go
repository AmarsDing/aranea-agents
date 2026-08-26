package biz

import (
	"context"

	"aranea-agents/internal/biz/session"
	"aranea-agents/internal/biz/tool"
)

// ---------------------------------------------------------------------------
// Team-layer narrow interfaces — extracted from concrete Usecase types so the
// team package depends only on the methods it (and its direct downstream
// consumers: agent.TRPCBuilderDeps) actually call.
// ---------------------------------------------------------------------------

// TeamUsageQuerier captures the subset of UsageUsecase needed by the team Runner.
// Stability:evolving
type TeamUsageQuerier interface {
	RecordTokenUsageEvent(ctx context.Context, e TokenUsageEvent) (TokenUsageEvent, error)
	// RecordAuxLLMUsage records auxiliary (non-turn) LLM usage, e.g. the team
	// intent pass (P1-2, 2026-08-19).
	RecordAuxLLMUsage(ctx context.Context, in AuxLLMUsageInput) error
	// QuoteTokenUsageCostMicroUSD computes the total cost (micro USD) for the
	// given token counts via the active pricing snapshot; 0 when unpriced.
	// Read-only (P2-1 TeamRunStep.CostMicroUSD 回填).
	QuoteTokenUsageCostMicroUSD(ctx context.Context, prov, mod string, inputTok, outputTok, cachedTok int) int64
}

// TeamSessionManager captures the subset of SessionUsecase needed by the team Runner.
// Superseded by SessionTurnWriterPort + SessionTurnExtrasPort in SessionTurnManager.
// Kept for the Runner's own session field (non-TurnDeps path).
// Stability:evolving
type TeamSessionManager interface {
	AppendChatMessage(ctx context.Context, sessionID string, msg ChatMessage, bumpModelCall bool) error
	AccumulateMetricsDelta(delta session.SessionMetricsDelta)
}

// TeamAgentLookup captures the subset of AgentUsecase needed by the team Runner
// and agent.TRPCBuilderDeps.
// Stability:evolving
type TeamAgentLookup interface {
	Get(ctx context.Context, id string) (Agent, error)
	GetEffectiveTools(ctx context.Context, agentID string) (AgentEffectiveTools, error)
	// BatchHydrateForBuild hydrates multiple agents for orchestration/build paths,
	// skipping extras queries that are only needed for list display.
	BatchHydrateForBuild(ctx context.Context, agents []Agent) ([]Agent, error)
}

// TeamToolLookup captures the subset of ToolUsecase needed by the team Runner
// (pass-through) and agent.TRPCBuilderDeps.
// Stability:evolving
type TeamToolLookup interface {
	GetTool(ctx context.Context, id string) (tool.Tool, error)
	// ListToolCatalogEntries batch-loads lightweight build-time catalog rows
	// in a single query, replacing per-key GetTool loops during agent build.
	ListToolCatalogEntries(ctx context.Context, keys []string) ([]tool.ToolCatalogEntry, error)
	ListToolAgentOverridesByAgent(ctx context.Context, agentID string) ([]tool.ToolAgentOverride, error)
	RecordToolInvocation(ctx context.Context, in tool.ToolInvocationWrite) error
	RecordToolInvocationParams(ctx context.Context, in tool.ToolInvocationParamWrite) error
	RecordToolInvocationAudit(ctx context.Context, in tool.ToolInvocationAuditWrite) error
	// HasToolGrant reports whether a persisted "always allow" grant exists
	// for the (agentID, toolKey) pair. Used by the confirmation decision
	// chain; store errors degrade to false (fail-closed).
	HasToolGrant(ctx context.Context, agentID, toolKey string) bool
	// GrantTool persists an "always allow" grant. Idempotent.
	GrantTool(ctx context.Context, agentID, toolKey, grantedBy string) error
	// ListEnabledParamRulesForGate returns enabled param rules for the
	// (canonicalized) tool key (79-runtime-governance R9 paramRuleGate).
	// Store 未装配或无规则均返回 nil, nil；查询错误由 gate fail-open。
	ListEnabledParamRulesForGate(ctx context.Context, toolKey string) ([]tool.ToolParamRule, error)
}

// TeamModelCatalog captures the subset of LlmProviderModelUsecase needed by the
// team Runner and agent.TRPCBuilderDeps.
// Stability:evolving
type TeamModelCatalog interface {
	GetByProviderAndModel(ctx context.Context, provider, model string) (ProviderModel, error)
	List(ctx context.Context) ([]ProviderModel, error)
}

// TeamSkillLookup captures the subset of SkillUsecase needed by the team Runner
// (pass-through) and agent.TRPCBuilderDeps. It also satisfies
// skillruntime.SkillResolver so it can be used directly in visibility filters.
// Stability:evolving
type TeamSkillLookup interface {
	ListEnabledPublishedSkillKeys(ctx context.Context) ([]string, error)
	ListEnabledPublishedSkillRefs(ctx context.Context) ([]SkillEnabledRef, error)
	ListEnabledPublishedSkillCandidates(ctx context.Context) ([]SkillRuntimeCandidate, error)
	ScoreByEmbedding(ctx context.Context, query string, candidates []SkillRuntimeCandidate) (map[string]float64, error)
	BatchGetSkillGuidance(ctx context.Context, slugs []string) ([]SkillGuidanceEntry, error)
	GetBySlug(ctx context.Context, slug string) (Skill, error)
	RecordInvocation(ctx context.Context, in SkillInvocationWrite) error
}

// CLIAdminSkillLister captures the subset of SkillUsecase needed by the
// cli_admin tool adapters in internal/service.
// Stability:evolving
type CLIAdminSkillLister interface {
	List(ctx context.Context, q SkillListQuery) (SkillListResult, error)
	Get(ctx context.Context, id string) (Skill, error)
	// GetBySlug resolves a skill by its slug (skill_key). Used by the
	// tool_assertion gate (F9, Phase 11) to verify install outcomes by key.
	GetBySlug(ctx context.Context, slug string) (Skill, error)
}

// CLIAdminAgentLister captures the subset of AgentUsecase needed by the
// cli_admin tool adapters in internal/service.
// Stability:evolving
type CLIAdminAgentLister interface {
	List(ctx context.Context, q AgentListQuery) (AgentListResult, error)
	Get(ctx context.Context, id string) (Agent, error)
	GetByAgentKey(ctx context.Context, agentKey string) (Agent, error)
}

// ---------------------------------------------------------------------------
// Compile-time assertions that concrete Usecase types satisfy the interfaces.
// ---------------------------------------------------------------------------

var _ TeamUsageQuerier = (*UsageUsecase)(nil)
var _ TeamSessionManager = (*SessionUsecase)(nil)
var _ TeamAgentLookup = (*AgentUsecase)(nil)
var _ TeamToolLookup = (*ToolUsecase)(nil)
var _ TeamModelCatalog = (*LlmProviderModelUsecase)(nil)
var _ TeamSkillLookup = (*SkillUsecase)(nil)
var _ CLIAdminSkillLister = (*SkillUsecase)(nil)
var _ CLIAdminAgentLister = (*AgentUsecase)(nil)
