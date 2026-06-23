package service

import (
	"context"
	"testing"
	"time"

	v1 "aranea-agents/api/kratos/skill_evolution/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

type stubProposalRepo struct {
	biz.SkillProposalReadWriter
	proposals map[string]biz.SkillProposal
}

func newStubProposalRepo() *stubProposalRepo {
	return &stubProposalRepo{proposals: make(map[string]biz.SkillProposal)}
}

func (s *stubProposalRepo) Create(_ context.Context, p biz.SkillProposal) (biz.SkillProposal, error) {
	s.proposals[p.ID] = p
	return p, nil
}

func (s *stubProposalRepo) GetByID(_ context.Context, id string) (biz.SkillProposal, error) {
	p, ok := s.proposals[id]
	if !ok {
		return biz.SkillProposal{}, apierror.NotFound("SKILL_EVO", "not found")
	}
	return p, nil
}

func (s *stubProposalRepo) GetByPatternHash(_ context.Context, agentID string, hash string) (*biz.SkillProposal, error) {
	for _, p := range s.proposals {
		if p.AgentID == agentID && p.PatternHash == hash {
			return &p, nil
		}
	}
	return nil, nil
}

func (s *stubProposalRepo) ListByAgent(_ context.Context, agentID string, status string, _ int, _ int) ([]biz.SkillProposal, error) {
	var result []biz.SkillProposal
	for _, p := range s.proposals {
		if (agentID == "" || p.AgentID == agentID) && (status == "" || string(p.Status) == status) {
			result = append(result, p)
		}
	}
	return result, nil
}

func (s *stubProposalRepo) CountByAgent(_ context.Context, agentID string, status string) (int, error) {
	count := 0
	for _, p := range s.proposals {
		if (agentID == "" || p.AgentID == agentID) && (status == "" || string(p.Status) == status) {
			count++
		}
	}
	return count, nil
}

func (s *stubProposalRepo) UpdateStatus(_ context.Context, id string, status biz.SkillProposalStatus, operator string) (biz.SkillProposal, error) {
	p, ok := s.proposals[id]
	if !ok {
		return biz.SkillProposal{}, apierror.NotFound("SKILL_EVO", "not found")
	}
	p.Status = status
	if status == biz.SkillProposalStatusApproved {
		now := time.Now().UTC()
		p.ApprovedAt = &now
		p.ApprovedBy = operator
	}
	if status == biz.SkillProposalStatusRejected {
		p.RejectedBy = operator
	}
	s.proposals[id] = p
	return p, nil
}

func newTestSkillEvolutionService(repo *stubProposalRepo, agents biz.AgentRepository) *SkillEvolutionService {
	uc := biz.NewSkillEvolutionUsecase(repo, nil, agents, nil, nil, loggateway.NewNoop())
	return NewSkillEvolutionService(uc, loggateway.NewNoop())
}

func TestSkillEvolutionService_ListSkillProposals(t *testing.T) {
	repo := newStubProposalRepo()
	repo.Create(context.Background(), biz.SkillProposal{
		ID: "p1", AgentID: "a1", Status: biz.SkillProposalStatusPending, CreatedAt: time.Now().UTC(),
	})
	repo.Create(context.Background(), biz.SkillProposal{
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
	repo.Create(context.Background(), biz.SkillProposal{
		ID: "p1", AgentID: "a1", Status: biz.SkillProposalStatusPending, CreatedAt: time.Now().UTC(),
	})
	repo.Create(context.Background(), biz.SkillProposal{
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
	repo.Create(context.Background(), biz.SkillProposal{
		ID: "p1", AgentID: "a1", Status: biz.SkillProposalStatusPending, CreatedAt: time.Now().UTC(),
	})
	repo.Create(context.Background(), biz.SkillProposal{
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
	repo.Create(context.Background(), biz.SkillProposal{
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
	repo.Create(context.Background(), biz.SkillProposal{
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
	repo.Create(context.Background(), biz.SkillProposal{
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
