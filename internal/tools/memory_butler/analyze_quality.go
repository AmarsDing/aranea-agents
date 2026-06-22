package memory_butler

import (
	"context"
	"time"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
	"trpc.group/trpc-go/trpc-agent-go/tool/function"
)

type analyzeMemoryQualityInput struct {
	AgentID string `json:"agent_id" jsonschema:"description=Agent ID,required"`
}

type analyzeMemoryQualityOutput struct {
	HitRate          float64 `json:"hit_rate"`
	MissRate         float64 `json:"miss_rate"`
	RedundancyScore  float64 `json:"redundancy_score"` // TODO(debt): not yet computed, always 0
	MisalignedCount  int     `json:"misaligned_count"`
	InactiveCount    int     `json:"inactive_count"`    // TODO(debt): not yet computed, always 0
	PredictableCount int     `json:"predictable_count"` // TODO(debt): not yet computed, always 0
	HealthScore      float64 `json:"health_score"`
}

func newAnalyzeMemoryQualityTool(deps Deps) trpctool.Tool {
	execute := func(ctx context.Context, input analyzeMemoryQualityInput) (analyzeMemoryQualityOutput, error) {
		if input.AgentID == "" {
			return analyzeMemoryQualityOutput{}, ErrAgentIDRequired
		}
		report, err := deps.Analytics.AnalyzeMemoryQuality(ctx, input.AgentID, time.Now().AddDate(0, 0, -30))
		if err != nil {
			return analyzeMemoryQualityOutput{}, err
		}
		return analyzeMemoryQualityOutput{
			HitRate:          report.RetrievalQuality,
			MissRate:         1.0 - report.RetrievalQuality,
			RedundancyScore:  0,
			MisalignedCount:  report.NegativeFeedback,
			InactiveCount:    0,
			PredictableCount: 0,
			HealthScore:      report.HealthScore,
		}, nil
	}
	return function.NewFunctionTool(
		execute,
		function.WithName("memory_butler_analyze_quality"),
		function.WithDescription("分析指定 Agent 的记忆质量，返回命中率、冗余度、未对齐数量、不活跃数量和健康评分。"),
	)
}
