package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	v1 "aranea-agents/api/kratos/skill_evolution/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// stubProposalRepo implements biz.UnifiedEvolutionStore over an in-memory map
// (A6). Proposals are stored as unified rows; the seed helper converts the
// legacy L1 view, mirroring unifiedFromSkillProposal in the biz layer.
type stubProposalRepo struct {
	proposals map[string]biz.UnifiedEvolutionSuggestion
}

func newStubProposalRepo() *stubProposalRepo {
	return &stubProposalRepo{proposals: make(map[string]biz.UnifiedEvolutionSuggestion)}
}

// proposalToUnified mirrors biz.unifiedFromSkillProposal for seeding.
func proposalToUnified(p biz.SkillProposal) biz.UnifiedEvolutionSuggestion {
	approvedAt := ""
	if p.ApprovedAt != nil {
		approvedAt = p.ApprovedAt.UTC().Format(time.RFC3339)
	}
	metadata, _ := json.Marshal(map[string]string{
		biz.EvoMetaPatternHash: p.PatternHash,
		biz.EvoMetaPatternDesc: p.PatternDesc,
		biz.EvoMetaApprovedAt:  approvedAt,
		biz.EvoMetaRejectedBy:  p.RejectedBy,
	})
	return biz.UnifiedEvolutionSuggestion{
		ID:              p.ID,
		TargetType:      biz.EvolutionTargetAgent,
		TargetID:        p.AgentID,
		ActionType:      biz.EvolutionActionCreate,
		TriggerSource:   "pattern",
		TriggerReason:   p.PatternDesc,
		Status:          string(p.Status),
		Priority:        1,
		DraftBody:       p.SkillMD,
		DraftName:       p.SkillName,
		LifecycleStatus: "draft",
		Metadata:        metadata,
		CreatedAt:       p.CreatedAt,
		ApprovedBy:      p.ApprovedBy,
	}
}

// seed inserts a proposal using the legacy L1 view (test helper).
func (s *stubProposalRepo) seed(p biz.SkillProposal) {
	s.proposals[p.ID] = proposalToUnified(p)
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

func newTestSkillEvolutionService(repo *stubProposalRepo, agents biz.AgentRepository) *SkillEvolutionService {
	uc := biz.NewSkillEvolutionUsecase(repo, nil, nil, agents, nil, nil, loggateway.NewNoop())
	return NewSkillEvolutionService(uc, loggateway.NewNoop())
}

func TestSkillEvolutionService_ListSkillProposals(t *testing.T) {
	repo := newStubProposalRepo()
	repo.seed(biz.SkillProposal{
		ID: "p1", AgentID: "a1", Status: biz.SkillProposalStatusPending, CreatedAt: time.Now().UTC(),
	})
	repo.seed(biz.SkillProposal{
		ID: "p2", AgentID: "a1", Status: biz.SkillProposalStatusApproved, CreatedAt: time.Now().UTC(),
	})

	svc := newTestSkillEvolutionService(repo, nil)
	resp, err := svc.ListSkillProposals(context.Background(), &v1.ListSkillProposalsRequest{AgentId: "a1"})
	if err != nil {
		t.Fatalf("ListSkillProposals: %v", err)
	}
	if len(resp.Items) != 2 {
		t.Errorf("expected 2 items, got %d", len(resp.Items))
	}
	if resp.Total != 2 {
		t.Errorf("expected total=2, got %d", resp.Total)
	}
}

func TestSkillEvolutionService_ListSkillProposals_FilterByStatus(t *testing.T) {
	repo := newStubProposalRepo()
	repo.seed(biz.SkillProposal{
		ID: "p1", AgentID: "a1", Status: biz.SkillProposalStatusPending, CreatedAt: time.Now().UTC(),
	})
	repo.seed(biz.SkillProposal{
		ID: "p2", AgentID: "a1", Status: biz.SkillProposalStatusApproved, CreatedAt: time.Now().UTC(),
	})

	svc := newTestSkillEvolutionService(repo, nil)
	resp, err := svc.ListSkillProposals(context.Background(), &v1.ListSkillProposalsRequest{AgentId: "a1", Status: "pending"})
	if err != nil {
		t.Fatalf("ListSkillProposals: %v", err)
	}
	if len(resp.Items) != 1 {
		t.Errorf("expected 1 pending item, got %d", len(resp.Items))
	}
	if resp.Items[0].Status != "pending" {
		t.Errorf("expected pending status, got %s", resp.Items[0].Status)
	}
}

func TestSkillEvolutionService_ListSkillProposals_EmptyAgentID(t *testing.T) {
	repo := newStubProposalRepo()
	repo.seed(biz.SkillProposal{
		ID: "p1", AgentID: "a1", Status: biz.SkillProposalStatusPending, CreatedAt: time.Now().UTC(),
	})
	repo.seed(biz.SkillProposal{
		ID: "p2", AgentID: "a2", Status: biz.SkillProposalStatusApproved, CreatedAt: time.Now().UTC(),
	})

	svc := newTestSkillEvolutionService(repo, nil)
	resp, err := svc.ListSkillProposals(context.Background(), &v1.ListSkillProposalsRequest{})
	if err != nil {
		t.Fatalf("ListSkillProposals with empty agentID: %v", err)
	}
	if len(resp.Items) != 2 {
		t.Errorf("expected 2 items (all agents), got %d", len(resp.Items))
	}
}

func TestSkillEvolutionService_GetSkillProposal(t *testing.T) {
	repo := newStubProposalRepo()
	repo.seed(biz.SkillProposal{
		ID: "p1", AgentID: "a1", SkillName: "test-skill", Status: biz.SkillProposalStatusPending, CreatedAt: time.Now().UTC(),
	})

	svc := newTestSkillEvolutionService(repo, nil)
	resp, err := svc.GetSkillProposal(context.Background(), &v1.GetSkillProposalRequest{Id: "p1"})
	if err != nil {
		t.Fatalf("GetSkillProposal: %v", err)
	}
	if resp.SkillName != "test-skill" {
		t.Errorf("expected test-skill, got %s", resp.SkillName)
	}
}

func TestSkillEvolutionService_GetSkillProposal_NotFound(t *testing.T) {
	repo := newStubProposalRepo()
	svc := newTestSkillEvolutionService(repo, nil)

	_, err := svc.GetSkillProposal(context.Background(), &v1.GetSkillProposalRequest{Id: "nonexistent"})
	if err == nil {
		t.Fatal("expected not found error")
	}
}

func TestSkillEvolutionService_ApproveSkillProposal(t *testing.T) {
	repo := newStubProposalRepo()
	repo.seed(biz.SkillProposal{
		ID: "p1", AgentID: "a1", Status: biz.SkillProposalStatusPending, CreatedAt: time.Now().UTC(),
	})

	svc := newTestSkillEvolutionService(repo, nil)
	resp, err := svc.ApproveSkillProposal(context.Background(), &v1.ApproveSkillProposalRequest{Id: "p1", ApprovedBy: "admin"})
	if err != nil {
		t.Fatalf("ApproveSkillProposal: %v", err)
	}
	if resp.Status != "approved" {
		t.Errorf("expected approved, got %s", resp.Status)
	}
}

func TestSkillEvolutionService_RejectSkillProposal(t *testing.T) {
	repo := newStubProposalRepo()
	repo.seed(biz.SkillProposal{
		ID: "p1", AgentID: "a1", Status: biz.SkillProposalStatusPending, CreatedAt: time.Now().UTC(),
	})

	svc := newTestSkillEvolutionService(repo, nil)
	resp, err := svc.RejectSkillProposal(context.Background(), &v1.RejectSkillProposalRequest{Id: "p1", RejectedBy: "admin"})
	if err != nil {
		t.Fatalf("RejectSkillProposal: %v", err)
	}
	if resp.Status != "rejected" {
		t.Errorf("expected rejected, got %s", resp.Status)
	}
}

func TestSkillEvolutionService_TriggerSkillDetection(t *testing.T) {
	repo := newStubProposalRepo()
	// DetectAndPropose fail-closes when agents==nil; provide a stub agent repo
	// so the test reaches the creator==nil path (0 proposals, no error).
	svc := newTestSkillEvolutionService(repo, channelTestAgentRepo{})

	resp, err := svc.TriggerSkillDetection(context.Background(), &v1.TriggerSkillDetectionRequest{AgentId: "a1"})
	if err != nil {
		t.Fatalf("TriggerSkillDetection: %v", err)
	}
	if len(resp.Proposals) != 0 {
		t.Errorf("expected 0 proposals without creator, got %d", len(resp.Proposals))
	}
}

func TestToProtoSkillProposal(t *testing.T) {
	now := time.Now().UTC()
	approvedAt := now.Add(time.Hour)
	p := biz.SkillProposal{
		ID:          "p1",
		AgentID:     "a1",
		PatternHash: "abc123",
		PatternDesc: "web_search(query)",
		SkillName:   "web-search",
		SkillMD:     "---\nname: web-search\n---\nbody",
		Status:      biz.SkillProposalStatusApproved,
		ApprovedBy:  "admin",
		CreatedAt:   now,
		ApprovedAt:  &approvedAt,
	}

	pb := toProtoSkillProposal(p)
	if pb.Id != "p1" {
		t.Errorf("expected id=p1, got %s", pb.Id)
	}
	if pb.AgentId != "a1" {
		t.Errorf("expected agent_id=a1, got %s", pb.AgentId)
	}
	if pb.Status != "approved" {
		t.Errorf("expected status=approved, got %s", pb.Status)
	}
	if pb.ApprovedBy != "admin" {
		t.Errorf("expected approved_by=admin, got %s", pb.ApprovedBy)
	}
	if pb.ApprovedAt == "" {
		t.Error("expected approved_at to be set")
	}
	if pb.CreatedAt == "" {
		t.Error("expected created_at to be set")
	}
}

func TestToProtoSkillProposal_NilApprovedAt(t *testing.T) {
	p := biz.SkillProposal{
		ID:        "p1",
		Status:    biz.SkillProposalStatusPending,
		CreatedAt: time.Now().UTC(),
	}

	pb := toProtoSkillProposal(p)
	if pb.ApprovedAt != "" {
		t.Errorf("expected empty approved_at for nil, got %s", pb.ApprovedAt)
	}
}
