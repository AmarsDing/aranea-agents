package cli_admin

import (
	"context"
	"fmt"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
	"trpc.group/trpc-go/trpc-agent-go/tool/function"
)

// ─── skill get ────────────────────────────────────────────────────────────────

type skillGetInput struct {
	ID string `json:"id" jsonschema:"description=Skill ID,required"`
}

func newSkillGetTool(deps Deps) trpctool.Tool {
	execute := func(ctx context.Context, input skillGetInput) (*SkillItem, error) {
		if input.ID == "" {
			return nil, fmt.Errorf("id is required")
		}
		return deps.SkillRepo.GetSkill(ctx, input.ID)
	}
	return function.NewFunctionTool(
		execute,
		function.WithName("cli_admin_skill_get"),
		function.WithDescription("获取指定 Skill 的详细信息。"),
	)
}

// ─── agent list ───────────────────────────────────────────────────────────────

type agentListInput struct {
	Keyword string `json:"keyword" jsonschema:"description=搜索关键词"`
	Limit   int32  `json:"limit" jsonschema:"description=最多返回条数,default=20"`
	Offset  int32  `json:"offset" jsonschema:"description=偏移量,default=0"`
}

type agentListOutput struct {
	Items []AgentItem `json:"items"`
	Total int32       `json:"total"`
}

func newAgentListTool(deps Deps) trpctool.Tool {
	execute := func(ctx context.Context, input agentListInput) (agentListOutput, error) {
		limit := input.Limit
		if limit <= 0 {
			limit = 20
		}
		items, total, err := deps.AgentRepo.ListAgents(ctx, input.Keyword, limit, input.Offset)
		if err != nil {
			return agentListOutput{}, err
		}
		return agentListOutput{Items: items, Total: total}, nil
	}
	return function.NewFunctionTool(
		execute,
		function.WithName("cli_admin_agent_list"),
		function.WithDescription("列出系统中所有 Agent。支持关键词搜索和分页。"),
	)
}

// ─── agent get ────────────────────────────────────────────────────────────────

type agentGetInput struct {
	ID string `json:"id" jsonschema:"description=Agent ID 或 agent_key,required"`
}

func newAgentGetTool(deps Deps) trpctool.Tool {
	execute := func(ctx context.Context, input agentGetInput) (*AgentItem, error) {
		if input.ID == "" {
			return nil, fmt.Errorf("id is required")
		}
		return deps.AgentRepo.GetAgent(ctx, input.ID)
	}
	return function.NewFunctionTool(
		execute,
		function.WithName("cli_admin_agent_get"),
		function.WithDescription("获取指定 Agent 的详细信息。"),
	)
}
