package service

import (
	"context"
	"testing"

	v1 "aranea-agents/api/kratos/skill_evolution_suggestion/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/apierror"
)

// E1（V 桶）pin：存量 approved 但未落 applied 的建议（approve 早于 E2E-E1
// 门禁，或 post-approve 异步沙箱未过）此前永久卡死——再点 approve 只撞 FSM
// pending-only 的 BadRequest。service 层把该形态解释为 re-validate 管理入口：
// 返回成功并异步重跑 沙箱重验 + apply 门禁（草稿按 F6 冻结）。

func TestApproveSkillEvolutionSuggestion_RevalidatesStuckApprovedRow(t *testing.T) {
	repo := newStubProposalRepo()
	// 卡死形态：status=approved + lifecycle=draft（未过 E2E-E1 门禁）。
	seedSkillSuggestionWithStatus(repo, "sug-stuck", "skill-1", string(biz.UnifiedEvolutionStateApproved))
	// curator/sandbox 为 nil → re-validate 分支不启异步管线，同步可断言。
	svc := newSkillSuggestionServiceWithSkill(repo, biz.Skill{ID: "skill-1", WorkspaceID: "ws-a"})

	ctx := workspace.WithContext(context.Background(), "ws-a")
	_, err := svc.ApproveSkillEvolutionSuggestion(ctx, &v1.ApproveSkillEvolutionSuggestionRequest{Id: "sug-stuck", ApprovedBy: "alice"})
	if err != nil {
		t.Fatalf("approve on a stuck approved row must be accepted as re-validate, got %v", err)
	}
	// 不重走 status 审批：FSM pending-only 的 CAS 不得被触碰。
	if got := repo.proposals["sug-stuck"].Status; got != string(biz.UnifiedEvolutionStateApproved) {
		t.Fatalf("status = %q, want approved (re-validate must not re-run the approval transition)", got)
	}
	if got := repo.proposals["sug-stuck"].ApprovedBy; got != "" {
		t.Fatalf("ApprovedBy = %q, want empty (no re-approval write)", got)
	}
}

// E1 pin：已落 applied 的建议不在卡死形态内——approve 仍走正常 FSM 路径并
// 拒绝（语义不回退，防止 re-validate 分支吞掉真正的非法迁移）。
func TestApproveSkillEvolutionSuggestion_AppliedRowStillRejected(t *testing.T) {
	repo := newStubProposalRepo()
	seedSkillSuggestionWithStatus(repo, "sug-done", "skill-1", string(biz.UnifiedEvolutionStateApplied))
	row := repo.proposals["sug-done"]
	row.LifecycleStatus = string(biz.EvoLifecycleApplied)
	repo.proposals["sug-done"] = row
	svc := newSkillSuggestionServiceWithSkill(repo, biz.Skill{ID: "skill-1", WorkspaceID: "ws-a"})

	ctx := workspace.WithContext(context.Background(), "ws-a")
	_, err := svc.ApproveSkillEvolutionSuggestion(ctx, &v1.ApproveSkillEvolutionSuggestionRequest{Id: "sug-done", ApprovedBy: "alice"})
	if !apierror.IsCode(err, apierror.CodeBadRequest) {
		t.Fatalf("approve on an applied row must still be rejected (BadRequest), got %v", err)
	}
}
