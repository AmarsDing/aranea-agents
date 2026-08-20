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

// stubProposalRepo implements biz.UnifiedEvolutionStore over an in-memory map
// (A6). 原定义在 skill_evolution_test.go，ADR-3-C5 删除 legacy 服务测试时
// 被一并移除；本文件（SkillEvolutionSuggestionService 测试）仍在使用，回迁于此。
type stubProposalRepo struct {
	proposals map[string]biz.UnifiedEvolutionSuggestion
}

func newStubProposalRepo() *stubProposalRepo {
	return &stubProposalRepo{proposals: make(map[string]biz.UnifiedEvolutionSuggestion)}
}

func (s *stubProposalRepo) Create(_ context.Context, u biz.UnifiedEvolutionSuggestion) error {
	s.proposals[u.ID] = u
	return nil
}

func (s *stubProposalRepo) GetByID(_ context.Context, id string) (*biz.UnifiedEvolutionSuggestion, error) {
	p, ok := s.proposals[id]
	if !ok {
		return nil, nil
	}
	return &p, nil
}

// mergeMeta sets keys on the row's JSON metadata, preserving existing keys.
func mergeMeta(raw json.RawMessage, kv map[string]string) json.RawMessage {
	m := map[string]string{}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &m)
	}
	for k, v := range kv {
		m[k] = v
	}
	out, _ := json.Marshal(m)
	return out
}

// UpdateStatus mirrors UnifiedEvolutionRepo.UpdateStatus metadata semantics.
func (s *stubProposalRepo) UpdateStatus(_ context.Context, id string, status string, actor string, reason string) error {
	p, ok := s.proposals[id]
	if !ok {
		return apierror.NotFound("SKILL_EVO", "not found")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	p.Status = status
	switch status {
	case string(biz.UnifiedEvolutionStateApproved):
		p.ApprovedBy = actor
		p.Metadata = mergeMeta(p.Metadata, map[string]string{
			biz.EvoMetaApprovedAt: now,
			biz.EvoMetaResolvedAt: now,
		})
	case string(biz.UnifiedEvolutionStateRejected):
		p.Metadata = mergeMeta(p.Metadata, map[string]string{
			biz.EvoMetaRejectedBy:      actor,
			biz.EvoMetaRejectionReason: reason,
			biz.EvoMetaResolvedAt:      now,
		})
	}
	s.proposals[id] = p
	return nil
}

// UpdateStatusCAS mirrors UnifiedEvolutionRepo.UpdateStatusCAS: update only when
// the current status is in from; report whether the transition happened.
func (s *stubProposalRepo) UpdateStatusCAS(ctx context.Context, id string, from []string, to string, actor string, reason string) (bool, error) {
	p, ok := s.proposals[id]
	if !ok {
		return false, apierror.NotFound("SKILL_EVO", "not found")
	}
	allowed := len(from) == 0
	for _, f := range from {
		if p.Status == f {
			allowed = true
			break
		}
	}
	if !allowed {
		return false, nil
	}
	if err := s.UpdateStatus(ctx, id, to, actor, reason); err != nil {
		return false, err
	}
	return true, nil
}

func (s *stubProposalRepo) filter(targetType, targetID, actionType, status string) []biz.UnifiedEvolutionSuggestion {
	var result []biz.UnifiedEvolutionSuggestion
	for _, p := range s.proposals {
		if targetType != "" && string(p.TargetType) != targetType {
			continue
		}
		if targetID != "" && p.TargetID != targetID {
			continue
		}
		if actionType != "" && string(p.ActionType) != actionType {
			continue
		}
		if status != "" && p.Status != status {
			continue
		}
		result = append(result, p)
	}
	return result
}

func (s *stubProposalRepo) ListByTarget(_ context.Context, targetType string, targetID string, _ string, status string, _, _ int) ([]biz.UnifiedEvolutionSuggestion, error) {
	return s.filter(targetType, targetID, "", status), nil
}

func (s *stubProposalRepo) CountByTarget(_ context.Context, targetType string, targetID string, _ string, status string) (int, error) {
	return len(s.filter(targetType, targetID, "", status)), nil
}

func (s *stubProposalRepo) ListByTargetAndAction(_ context.Context, targetType string, targetID string, actionType string, _ string, status string, _, _ int) ([]biz.UnifiedEvolutionSuggestion, error) {
	return s.filter(targetType, targetID, actionType, status), nil
}

func (s *stubProposalRepo) CountByTargetAndAction(_ context.Context, targetType string, targetID string, actionType string, _ string, status string) (int, error) {
	return len(s.filter(targetType, targetID, actionType, status)), nil
}

func (s *stubProposalRepo) HasPendingForTarget(_ context.Context, targetType string, targetID string) (bool, error) {
	return len(s.filter(targetType, targetID, "", "pending")) > 0, nil
}

func (s *stubProposalRepo) GetLatestByTarget(_ context.Context, targetType string, targetID string) (*biz.UnifiedEvolutionSuggestion, error) {
	var best *biz.UnifiedEvolutionSuggestion
	for i := range s.proposals {
		p := s.proposals[i]
		if targetType != "" && string(p.TargetType) != targetType {
			continue
		}
		if targetID != "" && p.TargetID != targetID {
			continue
		}
		if best == nil || p.CreatedAt.After(best.CreatedAt) {
			r := p
			best = &r
		}
	}
	return best, nil
}

func (s *stubProposalRepo) GetLatestByTargetAndAction(_ context.Context, targetType string, targetID string, actionType string) (*biz.UnifiedEvolutionSuggestion, error) {
	var best *biz.UnifiedEvolutionSuggestion
	for i := range s.proposals {
		p := s.proposals[i]
		if string(p.TargetType) != targetType || p.TargetID != targetID || string(p.ActionType) != actionType {
			continue
		}
		if best == nil || p.CreatedAt.After(best.CreatedAt) {
			r := p
			best = &r
		}
	}
	return best, nil
}

func (s *stubProposalRepo) UpdateDraftBody(_ context.Context, id string, draftBody string) error {
	p, ok := s.proposals[id]
	if !ok {
		return apierror.NotFound("SKILL_EVO", "not found")
	}
	p.DraftBody = draftBody
	s.proposals[id] = p
	return nil
}

func (s *stubProposalRepo) UpdateLifecycleStatus(context.Context, string, string) error {
	return nil
}

func (s *stubProposalRepo) UpdateSandboxResult(context.Context, string, bool, json.RawMessage) error {
	return nil
}

func (s *stubProposalRepo) UpdateMetadataKey(context.Context, string, string, string) error {
	return nil
}

func (s *stubProposalRepo) ExpireOlderThan(context.Context, time.Time) (int, error) {
	return 0, nil
}

func newSkillSuggestionServiceForTest(repo *stubProposalRepo) *SkillEvolutionSuggestionService {
	uc := biz.NewSkillIntelligenceUsecase(nil, nil, repo, nil, loggateway.NewNoop())
	curator := NewSkillCuratorService(uc, loggateway.NewNoop())
	return NewSkillEvolutionSuggestionService(uc, curator, nil, nil, nil, loggateway.NewNoop())
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
	skillUC := biz.NewSkillUsecase(&evoSkillGetterRepo{skill: sk}, nil, loggateway.NewNoop())
	return NewSkillEvolutionSuggestionService(uc, nil, nil, skillUC, nil, loggateway.NewNoop())
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
