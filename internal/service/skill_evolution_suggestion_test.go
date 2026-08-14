package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	v1 "aranea-agents/api/kratos/skill_evolution_suggestion/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// F6 (P-evo-2)：approve 后异步链不得覆盖审批时已有的草稿——仅当建议当前
// 无 DraftSkillBody 时才 GenerateDraft；沙盒校验与应用流程不受影响。

func newSkillSuggestionServiceForTest(repo *stubProposalRepo) *SkillEvolutionSuggestionService {
	uc := biz.NewSkillIntelligenceUsecase(nil, nil, repo, nil, loggateway.NewNoop())
	curator := NewSkillCuratorService(uc, loggateway.NewNoop())
	return NewSkillEvolutionSuggestionService(uc, curator, nil, nil, loggateway.NewNoop())
}

func seedSkillSuggestion(repo *stubProposalRepo, id, skillID, draft string) {
	metadata, _ := json.Marshal(map[string]string{
		biz.EvoMetaLegacyType: string(biz.EvoSuggestionFixFailure),
	})
	repo.proposals[id] = biz.UnifiedEvolutionSuggestion{
		ID:         id,
		TargetType: biz.EvolutionTargetSkill,
		TargetID:   skillID,
		ActionType: biz.EvolutionActionImprove,
		Status:     string(biz.UnifiedEvolutionStateApproved),
		DraftBody:  draft,
		Metadata:   metadata,
		CreatedAt:  time.Now().UTC(),
	}
}

func TestApproveSkillEvolutionSuggestion_KeepsExistingDraft(t *testing.T) {
	repo := newStubProposalRepo()
	const approvedDraft = "# Approved Draft\nhuman-reviewed body"
	seedSkillSuggestion(repo, "sug-1", "skill-1", approvedDraft)
	svc := newSkillSuggestionServiceForTest(repo)

	svc.runPostApproveAsync(context.Background(), "sug-1")

	if got := repo.proposals["sug-1"].DraftBody; got != approvedDraft {
		t.Fatalf("draft body overwritten by approve pipeline: got %q, want the approved draft frozen", got)
	}
}

func TestApproveSkillEvolutionSuggestion_GeneratesDraftWhenEmpty(t *testing.T) {
	repo := newStubProposalRepo()
	seedSkillSuggestion(repo, "sug-2", "skill-1", "")
	svc := newSkillSuggestionServiceForTest(repo)

	svc.runPostApproveAsync(context.Background(), "sug-2")

	if got := repo.proposals["sug-2"].DraftBody; got == "" {
		t.Fatal("draft body still empty after approve pipeline: want draft generated for draftless suggestion")
	}
}

// ── P0-1c: service 层 workspace IDOR 断言 ────────────────────────────────────

// evoSkillGetterRepo 仅实现 GetSkillByID，供 skill.Usecase.Get 断言路径使用。
type evoSkillGetterRepo struct {
	biz.SkillRepo
	skill biz.Skill
}

func (r *evoSkillGetterRepo) GetSkillByID(context.Context, string) (biz.Skill, error) {
	return r.skill, nil
}

func newSkillSuggestionServiceWithSkill(repo *stubProposalRepo, sk biz.Skill) *SkillEvolutionSuggestionService {
	uc := biz.NewSkillIntelligenceUsecase(nil, nil, repo, nil, loggateway.NewNoop())
	skillUC := biz.NewSkillUsecase(&evoSkillGetterRepo{skill: sk}, nil)
	return NewSkillEvolutionSuggestionService(uc, nil, nil, skillUC, loggateway.NewNoop())
}

func seedSkillSuggestionWithStatus(repo *stubProposalRepo, id, skillID, status string) {
	metadata, _ := json.Marshal(map[string]string{
		biz.EvoMetaLegacyType: string(biz.EvoSuggestionFixFailure),
	})
	repo.proposals[id] = biz.UnifiedEvolutionSuggestion{
		ID:         id,
		TargetType: biz.EvolutionTargetSkill,
		TargetID:   skillID,
		ActionType: biz.EvolutionActionImprove,
		Status:     status,
		Metadata:   metadata,
		CreatedAt:  time.Now().UTC(),
	}
}

func TestListSkillEvolutionSuggestions_CrossWorkspaceDenied(t *testing.T) {
	repo := newStubProposalRepo()
	seedSkillSuggestionWithStatus(repo, "sug-1", "skill-1", biz.EvolutionStatusPending)
	svc := newSkillSuggestionServiceWithSkill(repo, biz.Skill{ID: "skill-1", WorkspaceID: "ws-b"})

	ctx := workspace.WithContext(context.Background(), "ws-a")
	_, err := svc.ListSkillEvolutionSuggestions(ctx, &v1.ListSkillEvolutionSuggestionsRequest{SkillId: "skill-1"})
	if !apierror.IsCode(err, apierror.CodeNotFound) {
		t.Fatalf("cross-workspace list must return NotFound, got %v", err)
	}
}

func TestGetSkillEvolutionSuggestion_CrossWorkspaceDenied(t *testing.T) {
	repo := newStubProposalRepo()
	seedSkillSuggestionWithStatus(repo, "sug-1", "skill-1", biz.EvolutionStatusPending)
	svc := newSkillSuggestionServiceWithSkill(repo, biz.Skill{ID: "skill-1", WorkspaceID: "ws-b"})

	ctx := workspace.WithContext(context.Background(), "ws-a")
	_, err := svc.GetSkillEvolutionSuggestion(ctx, &v1.GetSkillEvolutionSuggestionRequest{Id: "sug-1"})
	if !apierror.IsCode(err, apierror.CodeNotFound) {
		t.Fatalf("cross-workspace get must return NotFound, got %v", err)
	}
}

func TestApproveSkillEvolutionSuggestion_CrossWorkspaceDenied(t *testing.T) {
	repo := newStubProposalRepo()
	seedSkillSuggestionWithStatus(repo, "sug-1", "skill-1", biz.EvolutionStatusPending)
	svc := newSkillSuggestionServiceWithSkill(repo, biz.Skill{ID: "skill-1", WorkspaceID: "ws-b"})

	ctx := workspace.WithContext(context.Background(), "ws-a")
	_, err := svc.ApproveSkillEvolutionSuggestion(ctx, &v1.ApproveSkillEvolutionSuggestionRequest{Id: "sug-1", ApprovedBy: "mallory"})
	if !apierror.IsCode(err, apierror.CodeNotFound) {
		t.Fatalf("cross-workspace approve must return NotFound, got %v", err)
	}
	if got := repo.proposals["sug-1"].Status; got != biz.EvolutionStatusPending {
		t.Fatalf("status must stay pending after denied approve, got %q", got)
	}
}

func TestRejectSkillEvolutionSuggestion_CrossWorkspaceDenied(t *testing.T) {
	repo := newStubProposalRepo()
	seedSkillSuggestionWithStatus(repo, "sug-1", "skill-1", biz.EvolutionStatusPending)
	svc := newSkillSuggestionServiceWithSkill(repo, biz.Skill{ID: "skill-1", WorkspaceID: "ws-b"})

	ctx := workspace.WithContext(context.Background(), "ws-a")
	_, err := svc.RejectSkillEvolutionSuggestion(ctx, &v1.RejectSkillEvolutionSuggestionRequest{Id: "sug-1", RejectedBy: "mallory"})
	if !apierror.IsCode(err, apierror.CodeNotFound) {
		t.Fatalf("cross-workspace reject must return NotFound, got %v", err)
	}
	if got := repo.proposals["sug-1"].Status; got != biz.EvolutionStatusPending {
		t.Fatalf("status must stay pending after denied reject, got %q", got)
	}
}

func TestTriggerCuratorFlow_CrossWorkspaceDenied(t *testing.T) {
	repo := newStubProposalRepo()
	svc := newSkillSuggestionServiceWithSkill(repo, biz.Skill{ID: "skill-1", WorkspaceID: "ws-b"})

	ctx := workspace.WithContext(context.Background(), "ws-a")
	_, err := svc.TriggerCuratorFlow(ctx, &v1.TriggerCuratorFlowRequest{SkillId: "skill-1"})
	if !apierror.IsCode(err, apierror.CodeNotFound) {
		t.Fatalf("cross-workspace trigger must return NotFound, got %v", err)
	}
}

func TestApproveSkillEvolutionSuggestion_SameWorkspaceAllowed(t *testing.T) {
	repo := newStubProposalRepo()
	seedSkillSuggestionWithStatus(repo, "sug-1", "skill-1", biz.EvolutionStatusPending)
	// curator/sandbox 为 nil：验证同步审批路径，避免异步 goroutine 与测试竞争。
	svc := newSkillSuggestionServiceWithSkill(repo, biz.Skill{ID: "skill-1", WorkspaceID: "ws-a"})

	ctx := workspace.WithContext(context.Background(), "ws-a")
	if _, err := svc.ApproveSkillEvolutionSuggestion(ctx, &v1.ApproveSkillEvolutionSuggestionRequest{Id: "sug-1", ApprovedBy: "alice"}); err != nil {
		t.Fatalf("same-workspace approve must succeed, got %v", err)
	}
	if got := repo.proposals["sug-1"].Status; got != string(biz.UnifiedEvolutionStateApproved) {
		t.Fatalf("status = %q, want approved", got)
	}
}
