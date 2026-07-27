package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// F6 (P-evo-2)：approve 后异步链不得覆盖审批时已有的草稿——仅当建议当前
// 无 DraftSkillBody 时才 GenerateDraft；沙盒校验与应用流程不受影响。

func newSkillSuggestionServiceForTest(repo *stubProposalRepo) *SkillEvolutionSuggestionService {
	uc := biz.NewSkillIntelligenceUsecase(nil, nil, repo, nil, loggateway.NewNoop())
	curator := NewSkillCuratorService(uc, loggateway.NewNoop())
	return NewSkillEvolutionSuggestionService(uc, curator, nil, loggateway.NewNoop())
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
