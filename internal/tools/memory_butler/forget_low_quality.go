package memory_butler

import (
	"context"

	"aranea-agents/pkg/jsonutil"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
	"trpc.group/trpc-go/trpc-agent-go/tool/function"
)

type forgetLowQualityInput struct {
	AgentID string `json:"agent_id" jsonschema:"description=Agent ID,required"`
	DryRun  bool   `json:"dry_run" jsonschema:"description=仅预览不实际删除,default=true"`
}

type forgetLowQualityOutput struct {
	DeletedCount int      `json:"deleted_count"`
	DeletedIDs   []string `json:"deleted_ids"`
}

func newForgetLowQualityTool(deps Deps) trpctool.Tool {
	execute := func(ctx context.Context, input forgetLowQualityInput) (forgetLowQualityOutput, error) {
		if input.AgentID == "" {
			return forgetLowQualityOutput{}, ErrAgentIDRequired
		}

		rows, _, _, _, err := deps.MemoryAdmin.ListFactRows(ctx, "agent", input.AgentID, "", "", "", 500, 0)
		if err != nil {
			return forgetLowQualityOutput{}, err
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
			hitCount := jsonutil.IfaceI32(m, "hit_count")
			negCount := jsonutil.IfaceI32(m, "negative_feedback_count")
			// A fact is considered misaligned if it has been retrieved enough times
			// (>=3) and has a high negative feedback rate (>50%).
			if hitCount >= 3 && negCount > 0 && float64(negCount)/float64(hitCount) > 0.5 {
				candidates = append(candidates, factID)
			}
		}

		if input.DryRun {
			return forgetLowQualityOutput{DeletedCount: len(candidates), DeletedIDs: candidates}, nil
		}

		if len(candidates) == 0 {
			return forgetLowQualityOutput{DeletedCount: 0, DeletedIDs: nil}, nil
		}

		deleted, err := deps.MemoryAdmin.DeleteFactRowsByIDs(ctx, candidates)
		if err != nil {
			return forgetLowQualityOutput{}, err
		}
		return forgetLowQualityOutput{DeletedCount: deleted, DeletedIDs: candidates}, nil
	}
	return function.NewFunctionTool(
		execute,
		function.WithName("memory_butler_forget_low_quality"),
		function.WithDescription("遗忘低质量记忆：识别并删除被多次检索但负面反馈率过高的记忆条目。支持 dry_run 模式预览。"),
	)
}
