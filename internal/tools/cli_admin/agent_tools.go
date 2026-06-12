package cli_admin

import (
	"context"
	"errors"

	"aranea-agents/pkg/apierror"
	kerrors "github.com/go-kratos/kratos/v2/errors"
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
			return nil, kerrors.BadRequest("CLI_ADMIN", "id is required")
		}
		return deps.SkillRepo.GetSkill(ctx, input.ID)
	}
	return function.NewFunctionTool(
		execute,
		function.WithName("cli_admin_skill_get"),
		function.WithDescription("获取指定 Skill 的详细信息，包含名称、描述、配置和关联的 Agent 列表。"),
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
		if input.Offset < 0 {
			return agentListOutput{}, kerrors.BadRequest("CLI_ADMIN", "offset must be >= 0")
		}
		limit := input.Limit
		if limit <= 0 {
			limit = 20
		}
		if limit > 100 {
			limit = 100
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
		function.WithDescription("列出系统中所有 Agent。支持关键词搜索和分页。用于查找可用的 Agent 或确认 Agent 是否存在。"),
	)
}

// ─── agent get ────────────────────────────────────────────────────────────────

type agentGetInput struct {
	ID string `json:"id" jsonschema:"description=Agent ID 或 agent_key,required"`
}

func newAgentGetTool(deps Deps) trpctool.Tool {
	execute := func(ctx context.Context, input agentGetInput) (*AgentItem, error) {
		if input.ID == "" {
			return nil, kerrors.BadRequest("CLI_ADMIN", "id is required")
		}
		item, err := deps.AgentRepo.GetAgent(ctx, input.ID)
		if err != nil {
			// Only fallback to agent_key lookup on NotFound errors.
			var ae *apierror.Error
			if !errors.As(err, &ae) || ae.Code != apierror.CodeNotFound {
				return nil, err
			}
			byKey, keyErr := deps.AgentRepo.GetAgentByAgentKey(ctx, input.ID)
			if keyErr != nil {
				return nil, err // return original error
			}
			return byKey, nil
		}
		return item, nil
	}
	return function.NewFunctionTool(
		execute,
		function.WithName("cli_admin_agent_get"),
		function.WithDescription("获取指定 Agent 的详细信息。支持按 ID 或 agent_key 查找，自动回退到 agent_key 查询。"),
	)
}
