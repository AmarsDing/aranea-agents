package skills_butler

import (
	"context"
	"crypto/sha256"
	"fmt"
	"time"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
	"trpc.group/trpc-go/trpc-agent-go/tool/function"
)

type evolveSkillInput struct {
	AgentID                string `json:"agent_id" jsonschema:"description=Agent ID,required"`
	SkillName              string `json:"skill_name" jsonschema:"description=要进化的 Skill 名称,required"`
	ImprovementDescription string `json:"improvement_description" jsonschema:"description=改进描述，说明需要优化的方向,required"`
}

type evolveSkillOutput struct {
	SuggestionID string `json:"suggestion_id"`
	SkillName    string `json:"skill_name"`
	Status       string `json:"status"`
	PatternDesc  string `json:"pattern_desc"`
	CreatedAt    string `json:"created_at"`
}

func newEvolveSkillTool(deps Deps) trpctool.Tool {
	execute := func(ctx context.Context, input evolveSkillInput) (evolveSkillOutput, error) {
		if input.AgentID == "" {
			return evolveSkillOutput{}, ErrAgentIDRequired
		}
		if input.SkillName == "" {
			return evolveSkillOutput{}, ErrSkillNameRequired
		}
		if input.ImprovementDescription == "" {
			return evolveSkillOutput{}, ErrImprovementDescRequired
		}
		patternDesc := fmt.Sprintf("%s: %s", input.SkillName, input.ImprovementDescription)
		h := sha256.Sum256([]byte(patternDesc))
		patternHash := fmt.Sprintf("%x", h[:8])
		// ADR-3: 统一进化建议入口（替代已退役的 SkillProposal 视图）。
		created, err := deps.Skills.CreateAgentSkillSuggestion(ctx, input.AgentID, input.SkillName, patternDesc, patternHash)
		if err != nil {
			return evolveSkillOutput{}, err
		}
		return evolveSkillOutput{
			SuggestionID: created.ID,
			SkillName:    created.DraftName,
			Status:       string(created.Status),
			PatternDesc:  patternDesc,
			CreatedAt:    created.CreatedAt.Format(time.RFC3339),
		}, nil
	}
	return function.NewFunctionTool(
		execute,
		function.WithName("skills_butler_evolve_skill"),
		function.WithDescription("为指定 Agent 创建 Skill 进化建议，描述改进方向后系统将生成待审批的建议。"),
	)
}
