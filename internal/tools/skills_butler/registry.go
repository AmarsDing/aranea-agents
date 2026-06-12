package skills_butler

import (
	"context"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// SkillUsecasePort manages skill proposals lifecycle.
// Stability:evolving
type SkillUsecasePort interface {
	ListProposals(ctx context.Context, agentID string, status string) ([]biz.SkillProposal, error)
	ApproveProposal(ctx context.Context, id string, approvedBy string) (biz.SkillProposal, error)
	RejectProposal(ctx context.Context, id string, rejectedBy string) (biz.SkillProposal, error)
	RegisterApproved(ctx context.Context, id string) (biz.SkillProposal, error)
	CreateProposal(ctx context.Context, proposal biz.SkillProposal) (biz.SkillProposal, error)
}

// EvolutionUsecasePort provides skill evolution metrics.
// Stability:evolving
type EvolutionUsecasePort interface {
	GetEvolutionMetrics(ctx context.Context, agentID string, timeRange string) (biz.EvolutionMetrics, error)
}

// SkillQueryReaderPort reads skill invocation statistics.
// Stability:evolving
type SkillQueryReaderPort interface {
	GetSkillInvocationStats(ctx context.Context, agentID string, since time.Time) ([]SkillInvocationStat, error)
}

// AnalyticsPort provides tool and skill analytics.
// Stability:evolving
type AnalyticsPort interface {
	AnalyzeToolWeights(ctx context.Context) ([]biz.ToolWeightReport, error)
	AnalyzeSkillHealth(ctx context.Context) ([]biz.SkillHealth, error)
	AnalyzeOrchestration(ctx context.Context, timeRange string, modeFilter string) ([]biz.OrchestrationModeReport, error)
}

type SkillInvocationStat struct {
	SkillName     string  `json:"skill_name"`
	Count         int     `json:"count"`
	SuccessRate   float64 `json:"success_rate"`
	AvgDurationMs int64   `json:"avg_duration_ms"`
}

type Deps struct {
	Skills    SkillUsecasePort
	Evolution EvolutionUsecasePort
	Queries   SkillQueryReaderPort
	Analytics AnalyticsPort
	LG        loggateway.Logger
}

func RegisterAll(deps Deps) []trpctool.Tool {
	tools := []trpctool.Tool{
		newAnalyzeSkillUsageTool(deps),
		newRecommendSkillsTool(deps),
		newEvolveSkillTool(deps),
		newOptimizeSkillTool(deps),
	}
	if deps.Analytics != nil {
		tools = append(tools,
			newAnalyzeSkillHealthTool(deps),
			newAnalyzeToolWeightsTool(deps),
			newAnalyzeOrchestrationTool(deps),
			newOptimizeOrchestrationTool(deps),
		)
	}
	return tools
}
