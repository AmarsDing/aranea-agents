package skills_butler

import (
	"context"
	"fmt"
	"time"

	"aranea-agents/pkg/loggateway"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
	"trpc.group/trpc-go/trpc-agent-go/tool/function"
)

type recommendSkillsInput struct {
	AgentID            string `json:"agent_id" jsonschema:"description=Agent ID,required"`
	ContextDescription string `json:"context_description" jsonschema:"description=当前场景描述，用于辅助推荐"`
}

type skillRecommendation struct {
	SkillName string `json:"skill_name"`
	Reason    string `json:"reason"`
	Source    string `json:"source"`
}

type recommendSkillsOutput struct {
	AgentID         string                `json:"agent_id"`
	Recommendations []skillRecommendation `json:"recommendations"`
}

func newRecommendSkillsTool(deps Deps) trpctool.Tool {
	execute := func(ctx context.Context, input recommendSkillsInput) (recommendSkillsOutput, error) {
		if input.AgentID == "" {
			return recommendSkillsOutput{}, ErrAgentIDRequired
		}
		lg := deps.LG
		if lg == nil {
			lg = loggateway.NewNoop()
		}
		var recs []skillRecommendation
		pendingProposals, err := deps.Skills.ListProposals(ctx, input.AgentID, "pending")
		if err != nil {
			lg.Warn("recommend_skills: ListProposals failed, skipping proposal-based recommendations",
				loggateway.StepID("skills_butler.recommend.list_proposals_fail"),
				loggateway.Str("agent_id", input.AgentID),
				loggateway.Err(err))
		} else {
			for _, p := range pendingProposals {
				recs = append(recs, skillRecommendation{
					SkillName: p.SkillName,
					Reason:    fmt.Sprintf("检测到重复工具调用模式：%s", p.PatternDesc),
					Source:    "pending_proposal",
				})
			}
		}
		since := time.Now().AddDate(0, 0, -30)
		stats, err := deps.Queries.GetSkillInvocationStats(ctx, input.AgentID, since)
		if err != nil {
			lg.Warn("recommend_skills: GetSkillInvocationStats failed, skipping usage-based recommendations",
				loggateway.StepID("skills_butler.recommend.stats_fail"),
				loggateway.Str("agent_id", input.AgentID),
				loggateway.Err(err))
		} else {
			for _, s := range stats {
				h := assessHealth(s, weeksPerMonth)
				if h == "warning" {
					recs = append(recs, skillRecommendation{
						SkillName: s.SkillName,
						Reason:    fmt.Sprintf("成功率偏低(%.0f%%)，建议优化或替换", s.SuccessRate*100),
						Source:    "usage_warning",
					})
				} else if h == "critical" {
					recs = append(recs, skillRecommendation{
						SkillName: s.SkillName,
						Reason:    fmt.Sprintf("使用率极低或成功率不足(%.0f%%)，建议移除或重构", s.SuccessRate*100),
						Source:    "usage_critical",
					})
				}
			}
		}
		return recommendSkillsOutput{
			AgentID:         input.AgentID,
			Recommendations: recs,
		}, nil
	}
	return function.NewFunctionTool(
		execute,
		function.WithName("skills_butler_recommend_skills"),
		function.WithDescription("基于 Agent 的 Skill 使用模式和进化建议，推荐新增、优化或移除的 Skill。"),
	)
}
