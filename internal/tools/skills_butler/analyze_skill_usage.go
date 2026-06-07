package skills_butler

import (
	"context"
	"time"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
	"trpc.group/trpc-go/trpc-agent-go/tool/function"
)

type analyzeSkillUsageInput struct {
	AgentID   string `json:"agent_id" jsonschema:"description=Agent ID,required"`
	TimeRange string `json:"time_range" jsonschema:"description=时间范围：7d/30d/90d,default=30d"`
}

type skillUsageItem struct {
	SkillName     string  `json:"skill_name"`
	Count         int     `json:"count"`
	SuccessRate   float64 `json:"success_rate"`
	AvgDurationMs int64   `json:"avg_duration_ms"`
	Health        string  `json:"health"`
}

type analyzeSkillUsageOutput struct {
	AgentID  string          `json:"agent_id"`
	TimeRange string         `json:"time_range"`
	Skills   []skillUsageItem `json:"skills"`
}

func newAnalyzeSkillUsageTool(deps Deps) trpctool.Tool {
	execute := func(ctx context.Context, input analyzeSkillUsageInput) (analyzeSkillUsageOutput, error) {
		if input.AgentID == "" {
			return analyzeSkillUsageOutput{}, ErrAgentIDRequired
		}
		since := timeRangeToSince(input.TimeRange)
		stats, err := deps.Queries.GetSkillInvocationStats(ctx, input.AgentID, since)
		if err != nil {
			return analyzeSkillUsageOutput{}, err
		}
		weeks := weeksInRange(input.TimeRange)
		var items []skillUsageItem
		for _, s := range stats {
			items = append(items, skillUsageItem{
				SkillName:     s.SkillName,
				Count:         s.Count,
				SuccessRate:   s.SuccessRate,
				AvgDurationMs: s.AvgDurationMs,
				Health:        assessHealth(s, weeks),
			})
		}
		return analyzeSkillUsageOutput{
			AgentID:   input.AgentID,
			TimeRange: input.TimeRange,
			Skills:    items,
		}, nil
	}
	return function.NewFunctionTool(
		execute,
		function.WithName("skills_butler_analyze_skill_usage"),
		function.WithDescription("分析指定 Agent 的 Skill 使用情况，返回每个 Skill 的调用次数、成功率、平均耗时和健康度评级。"),
	)
}

func assessHealth(stat SkillInvocationStat, weeks float64) string {
	callsPerWeek := float64(stat.Count) / weeks
	if callsPerWeek > 5 && stat.SuccessRate >= 0.8 {
		return "healthy"
	}
	if callsPerWeek > 5 && stat.SuccessRate >= 0.6 {
		return "warning"
	}
	if callsPerWeek < 2 || stat.SuccessRate < 0.6 {
		return "critical"
	}
	return "warning"
}

func timeRangeToSince(tr string) time.Time {
	now := time.Now()
	switch tr {
	case "7d":
		return now.AddDate(0, 0, -7)
	case "30d":
		return now.AddDate(0, 0, -30)
	case "90d":
		return now.AddDate(0, 0, -90)
	default:
		return now.AddDate(0, 0, -30)
	}
}

func weeksInRange(tr string) float64 {
	switch tr {
	case "7d":
		return 1.0
	case "30d":
		return 4.29
	case "90d":
		return 12.86
	default:
		return 4.29
	}
}
