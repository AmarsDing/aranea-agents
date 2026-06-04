package skills_butler

import (
	"context"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
	"trpc.group/trpc-go/trpc-agent-go/tool/function"
)

type analyzeSkillHealthInput struct {
	SkillID string `json:"skill_id" jsonschema:"description=要查询的 Skill ID，为空则返回全部"`
}

type skillHealthItem struct {
	SkillID        string  `json:"skill_id"`
	InvokeCount7d  int     `json:"invoke_count_7d"`
	SuccessRate    float64 `json:"success_rate"`
	AvgDurationMs  float64 `json:"avg_duration_ms"`
	Trend          string  `json:"trend"`
	HealthStatus   string  `json:"health_status"`
	Recommendation string  `json:"recommendation"`
}

type analyzeSkillHealthOutput struct {
	Skills []skillHealthItem `json:"skills"`
}

func newAnalyzeSkillHealthTool(deps Deps) trpctool.Tool {
	execute := func(ctx context.Context, input analyzeSkillHealthInput) (analyzeSkillHealthOutput, error) {
		reports, err := deps.Analytics.AnalyzeSkillHealth(ctx)
		if err != nil {
			return analyzeSkillHealthOutput{}, err
		}
		var items []skillHealthItem
		for _, r := range reports {
			if input.SkillID != "" && r.SkillID != input.SkillID {
				continue
			}
			items = append(items, skillHealthItem{
				SkillID:        r.SkillID,
				InvokeCount7d:  r.InvokeCount7d,
				SuccessRate:    r.SuccessRate,
				AvgDurationMs:  r.AvgDurationMS,
				Trend:          r.Trend,
				HealthStatus:   r.HealthStatus,
				Recommendation: r.Recommendation,
			})
		}
		return analyzeSkillHealthOutput{Skills: items}, nil
	}
	return function.NewFunctionTool(
		execute,
		function.WithName("skills_butler_analyze_skill_health"),
		function.WithDescription("分析所有 Skill 的健康状态，返回近 7 天调用次数、成功率、平均耗时、趋势和健康评级。"),
	)
}
