package memory_butler

import (
	"context"
	"errors"

	bizknowledge "aranea-agents/internal/biz/knowledge"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
	"trpc.group/trpc-go/trpc-agent-go/tool/function"
)

// 自治理知识图谱 M4.2：记忆管家接管知识库词条治理。
// 编排走 biz 层 CurateKnowledge（低风险自动应用、高风险 pending 提案人工二审）；
// 本工具仅做参数透传与报告投影，治理逻辑不下沉。

type knowledgeCurateInput struct {
	CollectionID string `json:"collection_id" jsonschema:"description=目标知识库集合ID（留空默认团队知识收件箱）"`
	DryRun       bool   `json:"dry_run" jsonschema:"description=仅预览不实际执行,default=true"`
}

type curateProposalItem struct {
	Kind     string `json:"kind"`
	Risk     string `json:"risk"`
	Status   string `json:"status"`
	DedupKey string `json:"dedup_key"`
}

type knowledgeCurateOutput struct {
	CollectionID      string               `json:"collection_id"`
	DryRun            bool                 `json:"dry_run"`
	DecayedEdges      int                  `json:"decayed_edges"`
	ClosedEdges       int                  `json:"closed_edges"`
	PromotedRelations []string             `json:"promoted_relations"`
	StaleMarked       int                  `json:"stale_marked"`
	ProposalsPending  int                  `json:"proposals_pending"`
	Proposals         []curateProposalItem `json:"proposals"`
	Actions           []string             `json:"actions"`
}

func newKnowledgeCurateTool(deps Deps) trpctool.Tool {
	execute := func(ctx context.Context, input knowledgeCurateInput) (knowledgeCurateOutput, error) {
		if deps.Knowledge == nil {
			return knowledgeCurateOutput{}, errors.New("knowledge curate not wired")
		}
		rep, err := deps.Knowledge.CurateKnowledge(ctx, bizknowledge.CurateOptions{
			CollectionID: input.CollectionID,
			DryRun:       input.DryRun,
		})
		if err != nil {
			return knowledgeCurateOutput{}, err
		}
		out := knowledgeCurateOutput{
			CollectionID:      rep.CollectionID,
			DryRun:            rep.DryRun,
			DecayedEdges:      rep.DecayedEdges,
			ClosedEdges:       rep.ClosedEdges,
			PromotedRelations: rep.PromotedRelations,
			StaleMarked:       rep.StaleMarked,
			ProposalsPending:  rep.ProposalsPending,
			Actions:           rep.Actions,
		}
		for _, p := range rep.Proposals {
			out.Proposals = append(out.Proposals, curateProposalItem{
				Kind: p.Kind, Risk: p.Risk, Status: p.Status, DedupKey: p.DedupKey,
			})
		}
		return out, nil
	}

	return function.NewFunctionTool(
		execute,
		function.WithName("memory_butler_knowledge_curate"),
		function.WithDescription("知识库词条治理：Hebbian 弱边衰减、陈旧词条标记、候选谓词提升自动执行；矛盾边与孤儿词条产高风险提案待人工二审。支持 dry_run 预览。"),
	)
}
