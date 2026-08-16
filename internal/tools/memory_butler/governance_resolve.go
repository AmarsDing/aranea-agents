package memory_butler

import (
	"context"
	"errors"
	"fmt"
	"strings"

	bizknowledge "aranea-agents/internal/biz/knowledge"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
	"trpc.group/trpc-go/trpc-agent-go/tool/function"
)

// 自治理知识图谱 M4 补丁：治理提案人工二审闭环（解析）。
// 高风险提案（conflict/orphan）只能 pending → applied/rejected，本工具是
// 唯一执行入口。**语义铁律：这是人工二审动作**——仅在用户明确指示批准或
// 拒绝某条提案时调用，禁止自主批量 resolve（否则高风险人工门禁形同虚设）。

type governanceResolveInput struct {
	ProposalID int64  `json:"proposal_id" jsonschema:"description=提案ID（从 governance_proposals 列表获取）,required"`
	Decision   string `json:"decision" jsonschema:"description=二审决定：applied=批准执行 / rejected=拒绝,required,enum=applied,enum=rejected"`
}

type governanceResolveOutput struct {
	ProposalID int64  `json:"proposal_id"`
	Decision   string `json:"decision"`
	Resolved   bool   `json:"resolved"`
}

func newGovernanceResolveTool(deps Deps) trpctool.Tool {
	execute := func(ctx context.Context, input governanceResolveInput) (governanceResolveOutput, error) {
		if deps.Knowledge == nil {
			return governanceResolveOutput{}, errors.New("knowledge curate not wired")
		}
		if input.ProposalID <= 0 {
			return governanceResolveOutput{}, fmt.Errorf("proposal_id must be positive, got %d", input.ProposalID)
		}
		decision := strings.ToLower(strings.TrimSpace(input.Decision))
		if decision != bizknowledge.ProposalStatusApplied && decision != bizknowledge.ProposalStatusRejected {
			return governanceResolveOutput{}, fmt.Errorf("decision must be %q or %q, got %q",
				bizknowledge.ProposalStatusApplied, bizknowledge.ProposalStatusRejected, input.Decision)
		}
		if err := deps.Knowledge.ResolveGovernanceProposal(ctx, input.ProposalID, decision); err != nil {
			return governanceResolveOutput{}, err
		}
		return governanceResolveOutput{
			ProposalID: input.ProposalID,
			Decision:   decision,
			Resolved:   true,
		}, nil
	}

	return function.NewFunctionTool(
		execute,
		function.WithName("memory_butler_governance_resolve"),
		function.WithDescription("人工二审知识库治理提案：applied=批准执行，rejected=拒绝。仅在用户明确指示时调用，禁止未经用户确认自主批量二审。"),
	)
}
