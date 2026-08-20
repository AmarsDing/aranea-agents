package skills_butler

import (
	"context"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// SkillUsecasePort manages agent-level skill-evolution suggestions via the
// ADR-3 unified model (the legacy SkillProposal view was retired; agent
// create_skill suggestions live in the unified evolution store).
// Stability:evolving
type SkillUsecasePort interface {
	ListAgentSuggestions(ctx context.Context, agentID string, status string) ([]biz.SkillEvolutionSuggestion, error)
	CreateAgentSkillSuggestion(ctx context.Context, agentID, skillName, patternDesc, patternHash string) (*biz.SkillEvolutionSuggestion, error)
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
