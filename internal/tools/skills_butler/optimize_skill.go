package skills_butler

import (
	"context"
	"fmt"
	"time"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
	"trpc.group/trpc-go/trpc-agent-go/tool/function"
)

type optimizeSkillInput struct {
	AgentID   string `json:"agent_id" jsonschema:"description=Agent ID,required"`
	SkillName string `json:"skill_name" jsonschema:"description=要优化的 Skill 名称,required"`
}

type optimizationSuggestion struct {
	Category    string `json:"category"`
	Description string `json:"description"`
	Priority    string `json:"priority"`
}

type optimizeSkillOutput struct {
	AgentID     string                 `json:"agent_id"`
	SkillName   string                 `json:"skill_name"`
	Health      string                 `json:"health"`
	Suggestions []optimizationSuggestion `json:"suggestions"`
}

func newOptimizeSkillTool(deps Deps) trpctool.Tool {
	execute := func(ctx context.Context, input optimizeSkillInput) (optimizeSkillOutput, error) {
		if input.AgentID == "" {
			return optimizeSkillOutput{}, errAgentIDRequired
		}
		if input.SkillName == "" {
			return optimizeSkillOutput{}, errSkillNameRequired
		}
		since := time.Now().AddDate(0, 0, -30)
		stats, err := deps.Queries.GetSkillInvocationStats(ctx, input.AgentID, since)
		if err != nil {
			return optimizeSkillOutput{}, err
		}
		var target *SkillInvocationStat
		for i := range stats {
			if stats[i].SkillName == input.SkillName {
				target = &stats[i]
				break
			}
		}
		if target == nil {
			return optimizeSkillOutput{
				AgentID:   input.AgentID,
				SkillName: input.SkillName,
				Health:    "unknown",
				Suggestions: []optimizationSuggestion{{
					Category:    "usage",
					Description: "该 Skill 在近 30 天内无调用记录，建议评估是否仍需要此 Skill",
					Priority:    "high",
				}},
			}, nil
		}
		health := assessHealth(*target, 4.29)
		var suggestions []optimizationSuggestion
		if target.SuccessRate < 0.6 {
			suggestions = append(suggestions, optimizationSuggestion{
				Category:    "reliability",
				Description: fmt.Sprintf("成功率仅 %.0f%%，建议检查 Skill 实现逻辑或调整工具参数", target.SuccessRate*100),
				Priority:    "high",
			})
		} else if target.SuccessRate < 0.8 {
			suggestions = append(suggestions, optimizationSuggestion{
				Category:    "reliability",
				Description: fmt.Sprintf("成功率 %.0f%%，存在优化空间，建议增加错误处理和重试逻辑", target.SuccessRate*100),
				Priority:    "medium",
			})
		}
		if target.AvgDurationMs > 5000 {
			suggestions = append(suggestions, optimizationSuggestion{
				Category:    "performance",
				Description: fmt.Sprintf("平均耗时 %dms，建议优化执行路径或引入缓存", target.AvgDurationMs),
				Priority:    "medium",
			})
		} else if target.AvgDurationMs > 2000 {
			suggestions = append(suggestions, optimizationSuggestion{
				Category:    "performance",
				Description: fmt.Sprintf("平均耗时 %dms，可考虑异步化或减少不必要的步骤", target.AvgDurationMs),
				Priority:    "low",
			})
		}
		callsPerWeek := float64(target.Count) / 4.29
		if callsPerWeek < 2 {
			suggestions = append(suggestions, optimizationSuggestion{
				Category:    "usage",
				Description: fmt.Sprintf("周均调用 %.1f 次，使用率极低，建议评估是否需要保留", callsPerWeek),
				Priority:    "medium",
			})
		}
		if len(suggestions) == 0 {
			suggestions = append(suggestions, optimizationSuggestion{
				Category:    "general",
				Description: "Skill 运行状况良好，暂无优化建议",
				Priority:    "low",
			})
		}
		return optimizeSkillOutput{
			AgentID:     input.AgentID,
			SkillName:   input.SkillName,
			Health:      health,
			Suggestions: suggestions,
		}, nil
	}
	return function.NewFunctionTool(
		execute,
		function.WithName("skills_butler_optimize_skill"),
		function.WithDescription("基于使用统计为指定 Skill 生成优化建议，包括可靠性、性能和使用率方面的改进方向。"),
	)
}
