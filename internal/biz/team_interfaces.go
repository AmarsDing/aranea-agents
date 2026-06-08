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
type TeamUsageQuerier interface {
	RecordTokenUsageEvent(ctx context.Context, e TokenUsageEvent) (TokenUsageEvent, error)
}

// TeamSessionManager captures the subset of SessionUsecase needed by the team Runner.
// Superseded by SessionTurnWriterPort + SessionTurnExtrasPort in SessionTurnManager.
// Kept for the Runner's own session field (non-TurnDeps path).
type TeamSessionManager interface {
	AppendChatMessage(ctx context.Context, sessionID string, msg ChatMessage, bumpModelCall bool) error
	AccumulateMetricsDelta(delta session.SessionMetricsDelta)
}

// TeamAgentLookup captures the subset of AgentUsecase needed by the team Runner
// and agent.TRPCBuilderDeps.
type TeamAgentLookup interface {
	Get(ctx context.Context, id string) (Agent, error)
	GetEffectiveTools(ctx context.Context, agentID string) (AgentEffectiveTools, error)
}

// TeamToolLookup captures the subset of ToolUsecase needed by the team Runner
// (pass-through) and agent.TRPCBuilderDeps.
type TeamToolLookup interface {
	GetTool(ctx context.Context, id string) (tool.Tool, error)
	ListToolAgentOverridesByAgent(ctx context.Context, agentID string) ([]tool.ToolAgentOverride, error)
	RecordToolInvocation(ctx context.Context, in tool.ToolInvocationWrite) error
	RecordToolInvocationAudit(ctx context.Context, in tool.ToolInvocationAuditWrite) error
}

// TeamModelCatalog captures the subset of LlmProviderModelUsecase needed by the
// team Runner and agent.TRPCBuilderDeps.
type TeamModelCatalog interface {
	GetByProviderAndModel(ctx context.Context, provider, model string) (ProviderModel, error)
	List(ctx context.Context) ([]ProviderModel, error)
}

// TeamSkillLookup captures the subset of SkillUsecase needed by the team Runner
// (pass-through) and agent.TRPCBuilderDeps. It also satisfies
// skillruntime.SkillResolver so it can be used directly in visibility filters.
type TeamSkillLookup interface {
	ListEnabledPublishedSkillKeys(ctx context.Context) ([]string, error)
	ListEnabledPublishedSkillCandidates(ctx context.Context) ([]SkillRuntimeCandidate, error)
	ScoreByEmbedding(ctx context.Context, query string, candidates []SkillRuntimeCandidate) (map[string]float64, error)
	BatchGetSkillGuidance(ctx context.Context, slugs []string) ([]SkillGuidanceEntry, error)
	GetBySlug(ctx context.Context, slug string) (Skill, error)
	RecordInvocation(ctx context.Context, in SkillInvocationWrite) error
}

// CLIAdminSkillLister captures the subset of SkillUsecase needed by the
// cli_admin tool adapters in internal/service.
type CLIAdminSkillLister interface {
	List(ctx context.Context, q SkillListQuery) (SkillListResult, error)
	Get(ctx context.Context, id string) (Skill, error)
}

// CLIAdminAgentLister captures the subset of AgentUsecase needed by the
// cli_admin tool adapters in internal/service.
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
