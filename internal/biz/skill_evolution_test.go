package biz

import (
	"context"
	"testing"
	"time"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

func isAPIErrorCode(err error, code apierror.Code) bool {
	ae, ok := apierror.From(err)
	return ok && ae.Code == code
}

type mockProposalRepo struct {
	SkillProposalReadWriter
	proposals map[string]SkillProposal
	byHash    map[string]*SkillProposal
	nextID    int
}

func newMockProposalRepo() *mockProposalRepo {
	return &mockProposalRepo{
		proposals: make(map[string]SkillProposal),
		byHash:    make(map[string]*SkillProposal),
	}
}

func (m *mockProposalRepo) Create(_ context.Context, p SkillProposal) (SkillProposal, error) {
	m.nextID++
	if p.ID == "" {
		p.ID = "prop-1"
	}
	m.proposals[p.ID] = p
	m.byHash[p.AgentID+":"+p.PatternHash] = &p
	return p, nil
}

func (m *mockProposalRepo) GetByID(_ context.Context, id string) (SkillProposal, error) {
	p, ok := m.proposals[id]
	if !ok {
		return SkillProposal{}, apierror.NotFound("SKILL_EVO", "proposal not found")
	}
	return p, nil
}

func (m *mockProposalRepo) GetByPatternHash(_ context.Context, agentID string, hash string) (*SkillProposal, error) {
	p, ok := m.byHash[agentID+":"+hash]
	if !ok {
		return nil, nil
	}
	return p, nil
}

func (m *mockProposalRepo) ListByAgent(_ context.Context, agentID string, status string, _ int, _ int) ([]SkillProposal, error) {
	var result []SkillProposal
	for _, p := range m.proposals {
		if (agentID == "" || p.AgentID == agentID) && (status == "" || string(p.Status) == status) {
			result = append(result, p)
		}
	}
	return result, nil
}

func (m *mockProposalRepo) CountByAgent(_ context.Context, agentID string, status string) (int, error) {
	count := 0
	for _, p := range m.proposals {
		if (agentID == "" || p.AgentID == agentID) && (status == "" || string(p.Status) == status) {
			count++
		}
	}
	return count, nil
}

func (m *mockProposalRepo) UpdateStatus(_ context.Context, id string, status SkillProposalStatus, operator string) (SkillProposal, error) {
	p, ok := m.proposals[id]
	if !ok {
		return SkillProposal{}, apierror.NotFound("SKILL_EVO", "proposal not found")
	}
	p.Status = status
	if status == SkillProposalStatusApproved {
		now := time.Now().UTC()
		p.ApprovedAt = &now
		p.ApprovedBy = operator
	}
	if status == SkillProposalStatusRejected {
		p.RejectedBy = operator
	}
	m.proposals[id] = p
	return p, nil
}

type mockPatternReader struct {
	PatternReader
	patterns []Pattern
}

func (m *mockPatternReader) ListByAgent(_ context.Context, agentID string, status string) ([]Pattern, error) {
	var result []Pattern
	for _, p := range m.patterns {
		if p.AgentID == agentID && string(p.Status) == status {
			result = append(result, p)
		}
	}
	return result, nil
}

type mockSkillAutoCreator struct {
	SkillAutoCreator
	name    string
	content string
	err     error
	called  int
}

func (m *mockSkillAutoCreator) GenerateSKILLMD(_ context.Context, _ string, _ []ToolCallRecord) (string, string, error) {
	m.called++
	return m.name, m.content, m.err
}

type mockSkillRegistrar struct {
	SkillRegistrationPort
	existing   map[string]bool
	registered map[string]string
	err        error
}

func (m *mockSkillRegistrar) RegisterSkill(_ context.Context, agentID string, name string, skillMD string) error {
	if m.err != nil {
		return m.err
	}
	if m.registered == nil {
		m.registered = make(map[string]string)
	}
	m.registered[agentID+":"+name] = skillMD
	return nil
}

func (m *mockSkillRegistrar) SkillExists(_ context.Context, agentID string, name string) (bool, error) {
	if m.err != nil {
		return false, m.err
	}
	return m.existing[agentID+":"+name], nil
}

func TestSkillEvolutionUsecase_RegisterApproved_EmptyMD(t *testing.T) {
	repo := newMockProposalRepo()
	registrar := &mockSkillRegistrar{existing: make(map[string]bool)}
	uc := NewSkillEvolutionUsecase(repo, nil, nil, nil, registrar, loggateway.NewNoop())

	repo.Create(context.Background(), SkillProposal{
		ID:        "p1",
		AgentID:   "a1",
		SkillName: "empty-skill",
		SkillMD:   "",
		Status:    SkillProposalStatusApproved,
	})

	_, err := uc.RegisterApproved(context.Background(), "p1")
	if err == nil {
		t.Fatal("expected error for empty SkillMD")
	}
	if !isAPIErrorCode(err, apierror.CodeBadRequest) {
		t.Errorf("expected BadRequest, got %v", err)
	}
}

func TestSkillEvolutionUsecase_RegisterApproved_NilRegistrar(t *testing.T) {
	repo := newMockProposalRepo()
	uc := NewSkillEvolutionUsecase(repo, nil, nil, nil, nil, loggateway.NewNoop())

	repo.Create(context.Background(), SkillProposal{
		ID:        "p1",
		AgentID:   "a1",
		SkillName: "my-skill",
		SkillMD:   "---\nname: my-skill\n---\nbody",
		Status:    SkillProposalStatusApproved,
	})

	result, err := uc.RegisterApproved(context.Background(), "p1")
	if err != nil {
		t.Fatalf("RegisterApproved with nil registrar: %v", err)
	}
	// nil registrar returns empty result (graceful degradation)
	if result.ID != "" {
		t.Errorf("expected empty result for nil registrar, got ID=%s", result.ID)
	}
}

func TestSkillEvolutionUsecase_ApproveProposal(t *testing.T) {
	repo := newMockProposalRepo()
	uc := NewSkillEvolutionUsecase(repo, nil, nil, nil, nil, loggateway.NewNoop())

	created, _ := repo.Create(context.Background(), SkillProposal{
		ID:        "p1",
		AgentID:   "a1",
		Status:    SkillProposalStatusPending,
		CreatedAt: time.Now().UTC(),
	})

	result, err := uc.ApproveProposal(context.Background(), created.ID, "user1")
	if err != nil {
		t.Fatalf("ApproveProposal: %v", err)
	}
	if result.Status != SkillProposalStatusApproved {
		t.Errorf("expected approved, got %s", result.Status)
	}
	if result.ApprovedBy != "user1" {
		t.Errorf("expected approved_by=user1, got %s", result.ApprovedBy)
	}
}

func TestSkillEvolutionUsecase_ApproveProposal_NotPending(t *testing.T) {
	repo := newMockProposalRepo()
	uc := NewSkillEvolutionUsecase(repo, nil, nil, nil, nil, loggateway.NewNoop())

	repo.Create(context.Background(), SkillProposal{
		ID:     "p1",
		Status: SkillProposalStatusApproved,
	})

	_, err := uc.ApproveProposal(context.Background(), "p1", "user1")
	if err == nil {
		t.Fatal("expected error for non-pending proposal")
	}
	if !isAPIErrorCode(err, apierror.CodeBadRequest) {
		t.Errorf("expected BadRequest, got %v", err)
	}
}

func TestSkillEvolutionUsecase_RejectProposal(t *testing.T) {
	repo := newMockProposalRepo()
	uc := NewSkillEvolutionUsecase(repo, nil, nil, nil, nil, loggateway.NewNoop())

	repo.Create(context.Background(), SkillProposal{
		ID:     "p1",
		Status: SkillProposalStatusPending,
	})

	result, err := uc.RejectProposal(context.Background(), "p1", "admin")
	if err != nil {
		t.Fatalf("RejectProposal: %v", err)
	}
	if result.Status != SkillProposalStatusRejected {
		t.Errorf("expected rejected, got %s", result.Status)
	}
	if result.RejectedBy != "admin" {
		t.Errorf("expected rejected_by=admin, got %s", result.RejectedBy)
	}
}

func TestSkillEvolutionUsecase_RegisterApproved(t *testing.T) {
	repo := newMockProposalRepo()
	registrar := &mockSkillRegistrar{existing: make(map[string]bool)}
	uc := NewSkillEvolutionUsecase(repo, nil, nil, nil, registrar, loggateway.NewNoop())

	repo.Create(context.Background(), SkillProposal{
		ID:        "p1",
		AgentID:   "a1",
		SkillName: "my-skill",
		SkillMD:   "---\nname: my-skill\n---\nbody",
		Status:    SkillProposalStatusApproved,
	})

	result, err := uc.RegisterApproved(context.Background(), "p1")
	if err != nil {
		t.Fatalf("RegisterApproved: %v", err)
	}
	if result.Status != SkillProposalStatusRegistered {
		t.Errorf("expected registered, got %s", result.Status)
	}
	if _, ok := registrar.registered["a1:my-skill"]; !ok {
		t.Error("expected skill to be registered")
	}
}

func TestSkillEvolutionUsecase_RegisterApproved_Conflict(t *testing.T) {
	repo := newMockProposalRepo()
	registrar := &mockSkillRegistrar{
		existing: map[string]bool{"a1:my-skill": true},
	}
	uc := NewSkillEvolutionUsecase(repo, nil, nil, nil, registrar, loggateway.NewNoop())

	repo.Create(context.Background(), SkillProposal{
		ID:        "p1",
		AgentID:   "a1",
		SkillName: "my-skill",
		SkillMD:   "---\nname: my-skill\n---\nbody",
		Status:    SkillProposalStatusApproved,
	})

	_, err := uc.RegisterApproved(context.Background(), "p1")
	if err == nil {
		t.Fatal("expected conflict error")
	}
	if !isAPIErrorCode(err, apierror.CodeConflict) {
		t.Errorf("expected Conflict, got %v", err)
	}
}

func TestSkillEvolutionUsecase_RegisterApproved_NotApproved(t *testing.T) {
	repo := newMockProposalRepo()
	uc := NewSkillEvolutionUsecase(repo, nil, nil, nil, nil, loggateway.NewNoop())

	repo.Create(context.Background(), SkillProposal{
		ID:     "p1",
		Status: SkillProposalStatusPending,
	})

	_, err := uc.RegisterApproved(context.Background(), "p1")
	if err == nil {
		t.Fatal("expected error for non-approved proposal")
	}
	if !isAPIErrorCode(err, apierror.CodeBadRequest) {
		t.Errorf("expected BadRequest, got %v", err)
	}
}

func TestSkillEvolutionUsecase_GetProposal(t *testing.T) {
	repo := newMockProposalRepo()
	uc := NewSkillEvolutionUsecase(repo, nil, nil, nil, nil, loggateway.NewNoop())

	repo.Create(context.Background(), SkillProposal{
		ID:        "p1",
		AgentID:   "a1",
		SkillName: "test-skill",
		Status:    SkillProposalStatusPending,
		CreatedAt: time.Now().UTC(),
	})

	result, err := uc.GetProposal(context.Background(), "p1")
	if err != nil {
		t.Fatalf("GetProposal: %v", err)
	}
	if result.SkillName != "test-skill" {
		t.Errorf("expected test-skill, got %s", result.SkillName)
	}
}

func TestSkillEvolutionUsecase_GetProposal_EmptyID(t *testing.T) {
	uc := NewSkillEvolutionUsecase(newMockProposalRepo(), nil, nil, nil, nil, loggateway.NewNoop())

	_, err := uc.GetProposal(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for empty id")
	}
}

func TestSkillEvolutionUsecase_ListProposals(t *testing.T) {
	repo := newMockProposalRepo()
	uc := NewSkillEvolutionUsecase(repo, nil, nil, nil, nil, loggateway.NewNoop())

	repo.Create(context.Background(), SkillProposal{ID: "p1", AgentID: "a1", Status: SkillProposalStatusPending})
	repo.Create(context.Background(), SkillProposal{ID: "p2", AgentID: "a1", Status: SkillProposalStatusApproved})
	repo.Create(context.Background(), SkillProposal{ID: "p3", AgentID: "a2", Status: SkillProposalStatusPending})

	all, err := uc.ListProposals(context.Background(), "a1", "", 0, 0)
	if err != nil {
		t.Fatalf("ListProposals: %v", err)
	}
	if len(all) != 2 {
		t.Errorf("expected 2 proposals, got %d", len(all))
	}

	pending, err := uc.ListProposals(context.Background(), "a1", "pending", 0, 0)
	if err != nil {
		t.Fatalf("ListProposals with status: %v", err)
	}
	if len(pending) != 1 {
		t.Errorf("expected 1 pending proposal, got %d", len(pending))
	}
}

func TestSkillEvolutionUsecase_ListProposals_EmptyAgentID(t *testing.T) {
	repo := newMockProposalRepo()
	uc := NewSkillEvolutionUsecase(repo, nil, nil, nil, nil, loggateway.NewNoop())

	repo.Create(context.Background(), SkillProposal{ID: "p1", AgentID: "a1", Status: SkillProposalStatusPending})
	repo.Create(context.Background(), SkillProposal{ID: "p2", AgentID: "a1", Status: SkillProposalStatusApproved})
	repo.Create(context.Background(), SkillProposal{ID: "p3", AgentID: "a2", Status: SkillProposalStatusPending})

	all, err := uc.ListProposals(context.Background(), "", "", 0, 0)
	if err != nil {
		t.Fatalf("ListProposals with empty agentID: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("expected 3 proposals (all agents), got %d", len(all))
	}
}

func TestSkillEvolutionUsecase_DetectAndPropose_NoCreator(t *testing.T) {
	uc := NewSkillEvolutionUsecase(newMockProposalRepo(), nil, nil, nil, nil, loggateway.NewNoop())

	proposals, err := uc.DetectAndPropose(context.Background(), "a1")
	if err != nil {
		t.Fatalf("DetectAndPropose: %v", err)
	}
	if len(proposals) != 0 {
		t.Errorf("expected 0 proposals without creator, got %d", len(proposals))
	}
}

func TestSkillEvolutionUsecase_DetectAndPropose_WithPatterns(t *testing.T) {
	repo := newMockProposalRepo()
	patterns := &mockPatternReader{
		patterns: []Pattern{
			{
				ID:          "pat1",
				AgentID:     "a1",
				Kind:        string(ObservationKindToolCall),
				Description: "web_search(query), summarize(text)",
				Confidence:  0.8,
				Status:      PatternStatusDetected,
				DetectedAt:  time.Now().UTC(),
			},
		},
	}
	creator := &mockSkillAutoCreator{name: "web-search-skill", content: "---\nname: web-search-skill\n---\nbody"}
	uc := NewSkillEvolutionUsecase(repo, patterns, nil, creator, nil, loggateway.NewNoop())

	proposals, err := uc.DetectAndPropose(context.Background(), "a1")
	if err != nil {
		t.Fatalf("DetectAndPropose: %v", err)
	}
	if len(proposals) != 1 {
		t.Fatalf("expected 1 proposal, got %d", len(proposals))
	}
	if proposals[0].SkillName != "web-search-skill" {
		t.Errorf("expected web-search-skill, got %s", proposals[0].SkillName)
	}
	if proposals[0].Status != SkillProposalStatusPending {
		t.Errorf("expected pending, got %s", proposals[0].Status)
	}
}

func TestSkillEvolutionUsecase_DetectAndPropose_DedupByHash(t *testing.T) {
	repo := newMockProposalRepo()
	patterns := &mockPatternReader{
		patterns: []Pattern{
			{
				ID:          "pat1",
				AgentID:     "a1",
				Kind:        string(ObservationKindToolCall),
				Description: "web_search(query)",
				Confidence:  0.8,
				Status:      PatternStatusDetected,
				DetectedAt:  time.Now().UTC(),
			},
		},
	}
	creator := &mockSkillAutoCreator{name: "web-search", content: "---\nname: web-search\n---\nbody"}
	uc := NewSkillEvolutionUsecase(repo, patterns, nil, creator, nil, loggateway.NewNoop())

	proposals1, _ := uc.DetectAndPropose(context.Background(), "a1")
	if len(proposals1) != 1 {
		t.Fatalf("first call: expected 1 proposal, got %d", len(proposals1))
	}

	proposals2, _ := uc.DetectAndPropose(context.Background(), "a1")
	if len(proposals2) != 0 {
		t.Errorf("second call: expected 0 proposals (dedup), got %d", len(proposals2))
	}
}

func TestSkillEvolutionUsecase_DetectAndPropose_LowConfidence(t *testing.T) {
	repo := newMockProposalRepo()
	patterns := &mockPatternReader{
		patterns: []Pattern{
			{
				ID:          "pat1",
				AgentID:     "a1",
				Kind:        string(ObservationKindToolCall),
				Description: "web_search(query)",
				Confidence:  0.1,
				Status:      PatternStatusDetected,
				DetectedAt:  time.Now().UTC(),
			},
		},
	}
	creator := &mockSkillAutoCreator{name: "web-search", content: "body"}
	uc := NewSkillEvolutionUsecase(repo, patterns, nil, creator, nil, loggateway.NewNoop())

	proposals, err := uc.DetectAndPropose(context.Background(), "a1")
	if err != nil {
		t.Fatalf("DetectAndPropose: %v", err)
	}
	if len(proposals) != 0 {
		t.Errorf("expected 0 proposals for low confidence, got %d", len(proposals))
	}
}

func TestSkillEvolutionUsecase_CreateProposal(t *testing.T) {
	repo := newMockProposalRepo()
	uc := NewSkillEvolutionUsecase(repo, nil, nil, nil, nil, loggateway.NewNoop())

	result, err := uc.CreateProposal(context.Background(), SkillProposal{
		AgentID:     "a1",
		PatternDesc: "web_search(query)",
		SkillName:   "web-search",
		SkillMD:     "body",
	})
	if err != nil {
		t.Fatalf("CreateProposal: %v", err)
	}
	if result.Status != SkillProposalStatusPending {
		t.Errorf("expected pending, got %s", result.Status)
	}
	if result.PatternHash == "" {
		t.Error("expected pattern hash to be auto-generated")
	}
	if result.ID == "" {
		t.Error("expected ID to be auto-generated")
	}
}

func TestPatternHash_Deterministic(t *testing.T) {
	h1 := patternHash("web_search(query)")
	h2 := patternHash("web_search(query)")
	if h1 != h2 {
		t.Errorf("patternHash should be deterministic: %s != %s", h1, h2)
	}

	h3 := patternHash("Web_Search(Query) ")
	if h1 != h3 {
		t.Errorf("patternHash should be case-insensitive and trim: %s != %s", h1, h3)
	}
}

func TestExtractToolNamesFromDesc(t *testing.T) {
	tests := []struct {
		desc string
		want []string
	}{
		{"web_search(query), summarize(text)", []string{"web_search", "summarize"}},
		{"single_tool(arg)", []string{"single_tool"}},
		{"no_parens", nil},
		{"", nil},
		{"tool1(a), tool2(b), tool3(c)", []string{"tool1", "tool2", "tool3"}},
	}

	for _, tt := range tests {
		got := extractToolNamesFromDesc(tt.desc)
		if len(got) != len(tt.want) {
			t.Errorf("extractToolNamesFromDesc(%q): got %v, want %v", tt.desc, got, tt.want)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("extractToolNamesFromDesc(%q)[%d]: got %q, want %q", tt.desc, i, got[i], tt.want[i])
			}
		}
	}
}
