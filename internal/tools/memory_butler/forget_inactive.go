package memory_butler

import (
	"context"
	"time"

	"aranea-agents/pkg/jsonutil"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
	"trpc.group/trpc-go/trpc-agent-go/tool/function"
)

type forgetInactiveInput struct {
	AgentID               string `json:"agent_id" jsonschema:"description=Agent ID,required"`
	InactiveThresholdDays int    `json:"inactive_threshold_days" jsonschema:"description=不活跃阈值天数,default=30"`
	DryRun                bool   `json:"dry_run" jsonschema:"description=仅预览不实际删除,default=true"`
}

type forgetInactiveOutput struct {
	ForgottenCount int      `json:"forgotten_count"`
	ForgottenIDs   []string `json:"forgotten_ids"`
}

func newForgetInactiveTool(deps Deps) trpctool.Tool {
	execute := func(ctx context.Context, input forgetInactiveInput) (forgetInactiveOutput, error) {
		if input.AgentID == "" {
			return forgetInactiveOutput{}, ErrAgentIDRequired
		}
		threshold := input.InactiveThresholdDays
		if threshold <= 0 {
			threshold = 30
		}
		cutoff := time.Now().UTC().AddDate(0, 0, -threshold)

		rows, _, _, _, err := deps.MemoryAdmin.ListFactRows(ctx, "agent", input.AgentID, "", "", "", 500, 0)
		if err != nil {
			return forgetInactiveOutput{}, err
		}

		var candidates []string
		for _, raw := range rows {
			m, _ := jsonutil.ParseMap(raw)
			if m == nil {
				continue
			}
			factID := jsonutil.IfaceStr(m, "id")
			if factID == "" {
				continue
			}
			updatedAt := jsonutil.IfaceStr(m, "updated_at")
			if updatedAt == "" {
				continue
			}
			t, parseErr := time.Parse(time.RFC3339, updatedAt)
			if parseErr != nil {
				continue
			}
			if t.Before(cutoff) {
				candidates = append(candidates, factID)
			}
		}

		if input.DryRun {
			return forgetInactiveOutput{ForgottenCount: len(candidates), ForgottenIDs: candidates}, nil
		}

		if len(candidates) == 0 {
			return forgetInactiveOutput{ForgottenCount: 0, ForgottenIDs: nil}, nil
		}

		deleted, err := deps.MemoryAdmin.DeleteFactRowsByIDs(ctx, candidates)
		if err != nil {
			return forgetInactiveOutput{}, err
		}
		return forgetInactiveOutput{ForgottenCount: deleted, ForgottenIDs: candidates}, nil
	}

	return function.NewFunctionTool(
		execute,
		function.WithName("memory_butler_forget_inactive"),
		function.WithDescription("遗忘不活跃记忆：识别并删除超过指定天数未更新的记忆条目。支持 dry_run 模式预览。"),
	)
}
