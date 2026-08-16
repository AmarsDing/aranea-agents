package memory_butler

import (
	"context"
	"errors"
	"time"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
	"trpc.group/trpc-go/trpc-agent-go/tool/function"
)

// 自治理知识图谱 M4 补丁：治理提案人工二审出口（列表）。
// 此前提案只写不读，pending 死信堆积无人可见；本工具把 biz 层
// ListGovernanceProposals 透传给记忆管家，供其向用户汇报待审提案。

type governanceProposalsInput struct {
	CollectionID string `json:"collection_id" jsonschema:"description=目标知识库集合ID（留空不过滤）"`
	Status       string `json:"status" jsonschema:"description=提案状态过滤：pending/applied/rejected（留空不过滤）,default=pending"`
	Limit        int    `json:"limit" jsonschema:"description=返回条数上限（默认50，最大200）,default=50"`
}

type governanceProposalItem struct {
	ID           int64          `json:"id"`
	CollectionID string         `json:"collection_id"`
	Kind         string         `json:"kind"`
	Risk         string         `json:"risk"`
	Status       string         `json:"status"`
	Payload      map[string]any `json:"payload"`
	CreatedAt    time.Time      `json:"created_at"`
	Resolved     bool           `json:"resolved"`
}

type governanceProposalsOutput struct {
	Proposals []governanceProposalItem `json:"proposals"`
	Total     int                      `json:"total"`
}

func newGovernanceProposalsTool(deps Deps) trpctool.Tool {
	execute := func(ctx context.Context, input governanceProposalsInput) (governanceProposalsOutput, error) {
		if deps.Knowledge == nil {
			return governanceProposalsOutput{}, errors.New("knowledge curate not wired")
		}
		views, err := deps.Knowledge.ListGovernanceProposals(ctx, input.CollectionID, input.Status, input.Limit)
		if err != nil {
			return governanceProposalsOutput{}, err
		}
		out := governanceProposalsOutput{Total: len(views)}
		for _, v := range views {
			out.Proposals = append(out.Proposals, governanceProposalItem{
				ID:           v.ID,
				CollectionID: v.CollectionID,
				Kind:         v.Kind,
				Risk:         v.Risk,
				Status:       v.Status,
				Payload:      v.Payload,
				CreatedAt:    v.CreatedAt,
				Resolved:     !v.ResolvedAt.IsZero(),
			})
		}
		return out, nil
	}

	return function.NewFunctionTool(
		execute,
		function.WithName("memory_butler_governance_proposals"),
		function.WithDescription("列出知识库治理提案（矛盾边/孤儿词条等高风险项），默认列 pending 待审清单。用于向用户汇报待人工二审的治理事项。"),
	)
}
