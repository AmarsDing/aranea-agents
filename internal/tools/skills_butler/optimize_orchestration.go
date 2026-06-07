package skills_butler

import (
	"context"
	"fmt"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
	"trpc.group/trpc-go/trpc-agent-go/tool/function"
)

type optimizeOrchestrationInput struct {
	TimeRange string `json:"time_range" jsonschema:"description=时间范围：7d/30d/90d,required"`
}

type orchestrationSuggestionItem struct {
	Type        string  `json:"type"`
	Description string  `json:"description"`
	Confidence  float64 `json:"confidence"`
}

type optimizeOrchestrationOutput struct {
	Suggestions []orchestrationSuggestionItem `json:"suggestions"`
}

func newOptimizeOrchestrationTool(deps Deps) trpctool.Tool {
	execute := func(ctx context.Context, input optimizeOrchestrationInput) (optimizeOrchestrationOutput, error) {
		if input.TimeRange == "" {
			return optimizeOrchestrationOutput{}, ErrTimeRangeRequired
		}
		reports, err := deps.Analytics.AnalyzeOrchestration(ctx, input.TimeRange, "")
		if err != nil {
			return optimizeOrchestrationOutput{}, err
		}
		if len(reports) == 0 {
			return optimizeOrchestrationOutput{Suggestions: []orchestrationSuggestionItem{}}, nil
		}

		var bestMode string
		var bestDQ float64
		for _, r := range reports {
			if r.DQScore > bestDQ {
				bestDQ = r.DQScore
				bestMode = r.Mode
			}
		}

		var suggestions []orchestrationSuggestionItem
		for _, r := range reports {
			if r.DQScore < 0.3 {
				suggestions = append(suggestions, orchestrationSuggestionItem{
					Type:        "avoid_topology",
					Description: fmt.Sprintf("编排模式 %s 的 DQ 评分仅 %.2f，建议标记该拓扑为 avoid", r.Mode, r.DQScore),
					Confidence:  0.9,
				})
			}
			if r.DQScore < 0.5 && bestMode != "" {
				suggestions = append(suggestions, orchestrationSuggestionItem{
					Type:        "switch_mode",
					Description: fmt.Sprintf("编排模式 %s 的 DQ 评分 %.2f 低于 0.5，建议切换到最佳模式 %s（DQ=%.2f）", r.Mode, r.DQScore, bestMode, bestDQ),
					Confidence:  0.8,
				})
			}
			if r.DQScore > 0.7 {
				suggestions = append(suggestions, orchestrationSuggestionItem{
					Type:        "cache_topology",
					Description: fmt.Sprintf("编排模式 %s 的 DQ 评分 %.2f 表现优秀，建议缓存该拓扑配置", r.Mode, r.DQScore),
					Confidence:  0.85,
				})
			}
		}

		return optimizeOrchestrationOutput{Suggestions: suggestions}, nil
	}
	return function.NewFunctionTool(
		execute,
		function.WithName("skills_butler_optimize_orchestration"),
		function.WithDescription("基于编排模式的 DQ 评分生成优化建议，包括模式切换、拓扑缓存和避免标记等操作建议。"),
	)
}
