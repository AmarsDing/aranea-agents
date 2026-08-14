package memory_butler

import (
	"context"
	"time"

	"aranea-agents/internal/biz"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
	"trpc.group/trpc-go/trpc-agent-go/tool/function"
)

type analyzeMemoryQualityInput struct {
	AgentID string `json:"agent_id" jsonschema:"description=Agent ID,required"`
}

type analyzeMemoryQualityOutput struct {
	HitRate          float64 `json:"hit_rate"`
	MissRate         float64 `json:"miss_rate"`
	RedundancyScore  float64 `json:"redundancy_score"` // near-duplicate fact fraction; 0 = none, not uncomputed
	MisalignedCount  int     `json:"misaligned_count"`
	InactiveCount    int     `json:"inactive_count"`    // facts idle >30d; 0 = all active, not uncomputed
	PredictableCount int     `json:"predictable_count"` // weaker near-duplicates; 0 = none, not uncomputed
	HealthScore      float64 `json:"health_score"`
}

func newAnalyzeMemoryQualityTool(deps Deps) trpctool.Tool {
	execute := func(ctx context.Context, input analyzeMemoryQualityInput) (analyzeMemoryQualityOutput, error) {
		if input.AgentID == "" {
			return analyzeMemoryQualityOutput{}, ErrAgentIDRequired
		}
		now := time.Now()
		report, err := deps.Analytics.AnalyzeMemoryQuality(ctx, input.AgentID, now.AddDate(0, 0, -30))
		if err != nil {
			return analyzeMemoryQualityOutput{}, err
		}
		rows, _, _, _, listErr := deps.MemoryAdmin.ListFactRows(ctx, biz.ListFactRowsParams{
			ScopeType: "agent",
			ScopeID:   input.AgentID,
			Limit:     defaultFactListLimit,
			Offset:    0,
		})
		if listErr != nil {
			return analyzeMemoryQualityOutput{}, listErr
		}
		metrics := computeQualityMetrics(parseQualityFacts(rows), now, defaultInactiveThresholdDays)
		return analyzeMemoryQualityOutput{
			HitRate:          report.RetrievalQuality,
			MissRate:         1.0 - report.RetrievalQuality,
			RedundancyScore:  metrics.RedundancyScore,
			MisalignedCount:  report.NegativeFeedback,
			InactiveCount:    metrics.InactiveCount,
			PredictableCount: metrics.PredictableCount,
			HealthScore:      report.HealthScore,
		}, nil
	}
	return function.NewFunctionTool(
		execute,
		function.WithName("memory_butler_analyze_quality"),
		function.WithDescription("分析指定 Agent 的记忆质量，返回命中率、冗余度（近重复事实占比，0 表示没有近重复）、未对齐数量、不活跃数量（超过 30 天未检索，0 表示全部活跃）和可预测数量（近重复对中的较弱项，0 表示没有可合并副本）。"),
	)
}
