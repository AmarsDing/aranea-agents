package skills_butler

import (
	"context"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
	"trpc.group/trpc-go/trpc-agent-go/tool/function"
)

type analyzeOrchestrationInput struct {
	TimeRange  string `json:"time_range" jsonschema:"description=时间范围：7d/30d/90d,default=30d"`
	ModeFilter string `json:"mode_filter" jsonschema:"description=按编排模式过滤，如 single/chain/cycle/parallel，为空则返回全部"`
}

type orchestrationModeItem struct {
	Mode                string             `json:"mode"`
	SuccessRate         float64            `json:"success_rate"`
	AvgTokens           int                `json:"avg_tokens"`
	AvgDurationSec      int                `json:"avg_duration_sec"`
	MemberContributions map[string]float64 `json:"member_contributions"`
	DQScore             float64            `json:"dq_score"`
}

type analyzeOrchestrationOutput struct {
	Modes []orchestrationModeItem `json:"modes"`
}

func newAnalyzeOrchestrationTool(deps Deps) trpctool.Tool {
	execute := func(ctx context.Context, input analyzeOrchestrationInput) (analyzeOrchestrationOutput, error) {
		timeRange := input.TimeRange
		if timeRange == "" {
			timeRange = "30d"
		}
		reports, err := deps.Analytics.AnalyzeOrchestration(ctx, timeRange, input.ModeFilter)
		if err != nil {
			return analyzeOrchestrationOutput{}, err
		}
		items := make([]orchestrationModeItem, 0, len(reports))
		for _, r := range reports {
			items = append(items, orchestrationModeItem{
				Mode:                r.Mode,
				SuccessRate:         r.SuccessRate,
				AvgTokens:           r.AvgTokens,
				AvgDurationSec:      r.AvgDurationSec,
				MemberContributions: r.MemberContributions,
				DQScore:             r.DQScore,
			})
		}
		return analyzeOrchestrationOutput{Modes: items}, nil
	}
	return function.NewFunctionTool(
		execute,
		function.WithName("skills_butler_analyze_orchestration"),
		function.WithDescription("分析编排模式的效果，返回各模式的成功率、平均 Token 消耗、平均耗时、成员贡献度和 DQ 评分。"),
	)
}
