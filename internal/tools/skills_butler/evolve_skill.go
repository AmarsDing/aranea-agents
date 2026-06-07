package skills_butler

import (
	"context"
	"crypto/sha256"
	"fmt"
	"time"

	"aranea-agents/internal/biz"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
	"trpc.group/trpc-go/trpc-agent-go/tool/function"
)

type evolveSkillInput struct {
	AgentID                string `json:"agent_id" jsonschema:"description=Agent ID,required"`
	SkillName              string `json:"skill_name" jsonschema:"description=要进化的 Skill 名称,required"`
	ImprovementDescription string `json:"improvement_description" jsonschema:"description=改进描述，说明需要优化的方向,required"`
}

type evolveSkillOutput struct {
	ProposalID  string `json:"proposal_id"`
	SkillName   string `json:"skill_name"`
	Status      string `json:"status"`
	PatternDesc string `json:"pattern_desc"`
	CreatedAt   string `json:"created_at"`
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
		proposal := biz.SkillProposal{
			AgentID:     input.AgentID,
			PatternHash: patternHash,
			PatternDesc: patternDesc,
			SkillName:   input.SkillName,
			SkillMD:     "",
			Status:      biz.SkillProposalStatusPending,
			CreatedAt:   time.Now().UTC(),
		}
		created, err := deps.Skills.CreateProposal(ctx, proposal)
		if err != nil {
			return evolveSkillOutput{}, err
		}
		return evolveSkillOutput{
			ProposalID:  created.ID,
			SkillName:   created.SkillName,
			Status:      string(created.Status),
			PatternDesc: created.PatternDesc,
			CreatedAt:   created.CreatedAt.Format(time.RFC3339),
		}, nil
	}
	return function.NewFunctionTool(
		execute,
		function.WithName("skills_butler_evolve_skill"),
		function.WithDescription("为指定 Agent 创建 Skill 进化提议，描述改进方向后系统将生成待审批的提案。"),
	)
}
