package memory_butler

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
	"trpc.group/trpc-go/trpc-agent-go/tool/function"
)

// 自治理知识图谱 M4 补丁：治理提案人工二审出口（列表）。
// 此前提案只写不读，pending 死信堆积无人可见；本工具把 biz 层
// ListGovernanceProposals 透传给记忆管家，供其向用户汇报待审提案。

// governancePayloadCap 单条提案 payload 序列化上限（字节）。moc_emerge 的
// members 全量数组可达数百词条（2026-08-18 域 D 评测 D19-b：23 条 pending 提案
// 输出撑爆 300s 客户端超时）；超限截断数组/长串并标注，保二审关键字段。
const governancePayloadCap = 2048

// compactGovernancePayload 超限 payload 压缩：数组成员保留前 8 个并附总数，
// 长字符串截断 300 字节。未超限原样返回（调用方只读）。
func compactGovernancePayload(p map[string]any) map[string]any {
	raw, err := json.Marshal(p)
	if err != nil || len(raw) <= governancePayloadCap {
		return p
	}
	out := make(map[string]any, len(p)+2)
	for k, v := range p {
		switch tv := v.(type) {
		case []any:
			if len(tv) > 8 {
				head := make([]any, 8)
				copy(head, tv[:8])
				out[k] = head
				out[k+"_total"] = len(tv)
				continue
			}
			out[k] = v
		case string:
			if len(tv) > 300 {
				out[k] = tv[:300] + "…"
				continue
			}
			out[k] = v
		default:
			out[k] = v
		}
	}
	out["_payload_truncated"] = true
	return out
}

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
				Payload:      compactGovernancePayload(v.Payload),
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
