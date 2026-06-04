package skills_butler

import (
	"context"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
	"trpc.group/trpc-go/trpc-agent-go/tool/function"
)

type analyzeToolWeightsInput struct {
	AgentID string `json:"agent_id" jsonschema:"description=Agent ID，为空则返回全部"`
}

type toolWeightItem struct {
	ToolKey        string  `json:"tool_key"`
	CallCount      int     `json:"call_count"`
	SuccessRate    float64 `json:"success_rate"`
	AvgDurationMs  float64 `json:"avg_duration_ms"`
	WeightScore    float64 `json:"weight_score"`
	Recommendation string  `json:"recommendation"`
}

type analyzeToolWeightsOutput struct {
	Tools []toolWeightItem `json:"tools"`
}

func newAnalyzeToolWeightsTool(deps Deps) trpctool.Tool {
	execute := func(ctx context.Context, input analyzeToolWeightsInput) (analyzeToolWeightsOutput, error) {
		reports, err := deps.Analytics.AnalyzeToolWeights(ctx)
		if err != nil {
			return analyzeToolWeightsOutput{}, err
		}
		items := make([]toolWeightItem, 0, len(reports))
		for _, r := range reports {
			items = append(items, toolWeightItem{
				ToolKey:        r.ToolKey,
				CallCount:      r.CallCount,
				SuccessRate:    r.SuccessRate,
				AvgDurationMs:  r.AvgDurationMS,
				WeightScore:    r.WeightScore,
				Recommendation: r.Recommendation,
			})
		}
		return analyzeToolWeightsOutput{Tools: items}, nil
	}
	return function.NewFunctionTool(
		execute,
		function.WithName("skills_butler_analyze_tool_weights"),
		function.WithDescription("分析所有工具的权重分布，返回调用次数、成功率、平均耗时、权重评分和推荐操作。"),
	)
}
