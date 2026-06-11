package cli_admin

import (
	"context"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
	"trpc.group/trpc-go/trpc-agent-go/tool/function"
)

type skillListInput struct {
	Keyword  string `json:"keyword" jsonschema:"description=搜索关键词"`
	Limit    int32  `json:"limit" jsonschema:"description=最多返回条数,default=20"`
	Offset   int32  `json:"offset" jsonschema:"description=偏移量,default=0"`
}

type skillListOutput struct {
	Items []SkillItem `json:"items"`
	Total int32       `json:"total"`
}

func newSkillListTool(deps Deps) trpctool.Tool {
	execute := func(ctx context.Context, input skillListInput) (skillListOutput, error) {
		limit := input.Limit
		if limit <= 0 {
			limit = 20
		}
		items, total, err := deps.SkillRepo.ListSkills(ctx, input.Keyword, limit, input.Offset)
		if err != nil {
			return skillListOutput{}, err
		}
		return skillListOutput{Items: items, Total: total}, nil
	}
	return function.NewFunctionTool(
		execute,
		function.WithName("cli_admin_skill_list"),
		function.WithDescription("列出系统中所有 Skill。支持关键词搜索和分页。用于查找可用的 Skill 或确认 Skill 是否已安装。"),
	)
}
