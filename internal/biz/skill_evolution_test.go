package biz

import (
	"context"
	"encoding/json"
	"sort"
	"testing"
	"time"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

func isAPIErrorCode(err error, code apierror.Code) bool {
	ae, ok := apierror.From(err)
	return ok && ae.Code == code
}

// mockUnifiedEvolutionStore is an in-memory UnifiedEvolutionStore +
// UnifiedEvolutionPatternReader for L1 proposal tests (A6). Unexercised
// interface methods panic via the embedded nil interfaces.
type mockUnifiedEvolutionStore struct {
	UnifiedEvolutionStore
	UnifiedEvolutionPatternReader
	rows map[string]UnifiedEvolutionSuggestion
	// casForceMiss simulates a concurrent transition winning the race between
	// the usecase's GetByID and the atomic UPDATE: UpdateStatusCAS returns
	// ok=false so tests can assert the Conflict mapping.
	casForceMiss bool
}

func newMockUnifiedEvolutionStore() *mockUnifiedEvolutionStore {
	return &mockUnifiedEvolutionStore{rows: make(map[string]UnifiedEvolutionSuggestion)}
}

// seed inserts an L1 proposal as a unified row (test helper mirroring the
// 20261111 backfill mapping).
func (m *mockUnifiedEvolutionStore) seed(p SkillProposal) {
	row := unifiedFromSkillProposal(p)
	m.rows[row.ID] = row
}

func metaJSONString(s string) json.RawMessage {
	b, _ := json.Marshal(s)
	return b
}

func (m *mockUnifiedEvolutionStore) GetByID(_ context.Context, id string) (*UnifiedEvolutionSuggestion, error) {
	row, ok := m.rows[id]
	if !ok {
		return nil, nil
	}
	return &row, nil
}

func (m *mockUnifiedEvolutionStore) ListByTargetAndAction(_ context.Context, targetType string, targetID string, actionType string, _ string, status string, _ int, _ int) ([]UnifiedEvolutionSuggestion, error) {
	var out []UnifiedEvolutionSuggestion
	for _, r := range m.rows {
		if r.TargetType != EvolutionTargetType(targetType) {
			continue
		}
		if targetID != "" && r.TargetID != targetID {
			continue
		}
		if actionType != "" && r.ActionType != EvolutionActionType(actionType) {
			continue
		}
		if status != "" && r.Status != status {
			continue
		}
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (m *mockUnifiedEvolutionStore) CountByTargetAndAction(_ context.Context, targetType string, targetID string, actionType string, _ string, status string) (int, error) {
	rows, err := m.ListByTargetAndAction(context.Background(), targetType, targetID, actionType, "", status, 0, 0)
	return len(rows), err
}

func (m *mockUnifiedEvolutionStore) GetLatestByPatternHash(_ context.Context, agentID string, patternHash string) (*UnifiedEvolutionSuggestion, error) {
	for _, r := range m.rows {
		if r.TargetType == EvolutionTargetAgent && r.TargetID == agentID &&
			r.ActionType == EvolutionActionCreate && r.MetaString(EvoMetaPatternHash) == patternHash {
			row := r
			return &row, nil
		}
	}
	return nil, nil
}

func (m *mockUnifiedEvolutionStore) Create(_ context.Context, s UnifiedEvolutionSuggestion) error {
	m.rows[s.ID] = s
	return nil
}

// UpdateStatus mirrors the real repo's metadata merge semantics so the L1
// view layer can reconstruct approved_by/at and rejected_by (A6).
func (m *mockUnifiedEvolutionStore) UpdateStatus(_ context.Context, id string, status string, actor string, reason string) error {
	return m.applyStatusMeta(id, status, actor, reason)
}

// UpdateStatusCAS mirrors the repo's atomic precondition: ok=false when the
// row's current status is not in `from` (a concurrent transition won), or when
// casForceMiss is set. Successful CAS applies the same metadata merge as
// UpdateStatus.
func (m *mockUnifiedEvolutionStore) UpdateStatusCAS(_ context.Context, id string, from []string, status string, actor string, reason string) (bool, error) {
	if m.casForceMiss {
		return false, nil
	}
	row, ok := m.rows[id]
	if !ok {
		return false, apierror.NotFound("SKILL_EVO", "proposal not found")
	}
	if len(from) > 0 {
		match := false
		for _, f := range from {
			if row.Status == f {
				match = true
				break
			}
		}
		if !match {
			return false, nil
		}
	}
	return true, m.applyStatusMeta(id, status, actor, reason)
}

func (m *mockUnifiedEvolutionStore) applyStatusMeta(id string, status string, actor string, reason string) error {
	row, ok := m.rows[id]
	if !ok {
		return apierror.NotFound("SKILL_EVO", "proposal not found")
	}
	row.Status = status
	meta := row.MetadataMap()
	now := time.Now().UTC().Format(time.RFC3339)
	switch status {
	case string(UnifiedEvolutionStateApproved):
		row.ApprovedBy = actor
		meta[EvoMetaApprovedAt] = metaJSONString(now)
		meta[EvoMetaResolvedAt] = metaJSONString(now)
	case string(UnifiedEvolutionStateRejected):
		row.ApprovedBy = actor
		meta[EvoMetaRejectedBy] = metaJSONString(actor)
		meta[EvoMetaRejectionReason] = metaJSONString(reason)
		meta[EvoMetaResolvedAt] = metaJSONString(now)
	case string(UnifiedEvolutionStateApplied):
		t := time.Now().UTC()
		row.AppliedAt = &t
	}
	row.Metadata, _ = json.Marshal(meta)
	m.rows[id] = row
	return nil
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
	repo := newMockUnifiedEvolutionStore()
	registrar := &mockSkillRegistrar{existing: make(map[string]bool)}
	uc := NewSkillEvolutionUsecase(repo, repo, nil, nil, nil, registrar, loggateway.NewNoop())

	repo.seed(SkillProposal{
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
	repo := newMockUnifiedEvolutionStore()
	uc := NewSkillEvolutionUsecase(repo, repo, nil, nil, nil, nil, loggateway.NewNoop())

	repo.seed(SkillProposal{
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
	repo := newMockUnifiedEvolutionStore()
	uc := NewSkillEvolutionUsecase(repo, repo, nil, nil, nil, nil, loggateway.NewNoop())

	repo.seed(SkillProposal{
		ID:        "p1",
		AgentID:   "a1",
		Status:    SkillProposalStatusPending,
		CreatedAt: time.Now().UTC(),
	})

	result, err := uc.ApproveProposal(context.Background(), "p1", "user1")
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
	repo := newMockUnifiedEvolutionStore()
	uc := NewSkillEvolutionUsecase(repo, repo, nil, nil, nil, nil, loggateway.NewNoop())

	repo.seed(SkillProposal{
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
	repo := newMockUnifiedEvolutionStore()
	uc := NewSkillEvolutionUsecase(repo, repo, nil, nil, nil, nil, loggateway.NewNoop())

	repo.seed(SkillProposal{
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

// B-1: CAS miss (concurrent transition won between GetByID and the atomic
// UPDATE) must surface as Conflict, never as a silent overwrite.
func TestSkillEvolutionUsecase_ApproveProposal_CASConflict(t *testing.T) {
	repo := newMockUnifiedEvolutionStore()
	repo.casForceMiss = true
	uc := NewSkillEvolutionUsecase(repo, repo, nil, nil, nil, nil, loggateway.NewNoop())

	repo.seed(SkillProposal{
		ID:        "p1",
		AgentID:   "a1",
		Status:    SkillProposalStatusPending,
		CreatedAt: time.Now().UTC(),
	})

	_, err := uc.ApproveProposal(context.Background(), "p1", "user1")
	if !isAPIErrorCode(err, apierror.CodeConflict) {
		t.Fatalf("expected Conflict on CAS miss, got %v", err)
	}
	if repo.rows["p1"].Status != string(UnifiedEvolutionStatePending) {
		t.Errorf("row status must remain untouched on CAS miss, got %s", repo.rows["p1"].Status)
	}
}

func TestSkillEvolutionUsecase_RejectProposal_CASConflict(t *testing.T) {
	repo := newMockUnifiedEvolutionStore()
	repo.casForceMiss = true
	uc := NewSkillEvolutionUsecase(repo, repo, nil, nil, nil, nil, loggateway.NewNoop())

	repo.seed(SkillProposal{
		ID:     "p1",
		Status: SkillProposalStatusPending,
	})

	_, err := uc.RejectProposal(context.Background(), "p1", "admin")
	if !isAPIErrorCode(err, apierror.CodeConflict) {
		t.Fatalf("expected Conflict on CAS miss, got %v", err)
	}
}

func TestSkillEvolutionUsecase_RegisterApproved_CASConflict(t *testing.T) {
	repo := newMockUnifiedEvolutionStore()
	registrar := &mockSkillRegistrar{existing: make(map[string]bool)}
	uc := NewSkillEvolutionUsecase(repo, repo, nil, nil, nil, registrar, loggateway.NewNoop())

	repo.seed(SkillProposal{
		ID:        "p1",
		AgentID:   "a1",
		SkillName: "my-skill",
		SkillMD:   "---\nname: my-skill\n---\nbody",
		Status:    SkillProposalStatusApproved,
	})
	repo.casForceMiss = true

	_, err := uc.RegisterApproved(context.Background(), "p1")
	if !isAPIErrorCode(err, apierror.CodeConflict) {
		t.Fatalf("expected Conflict on CAS miss, got %v", err)
	}
}

func TestSkillEvolutionUsecase_RegisterApproved(t *testing.T) {
	repo := newMockUnifiedEvolutionStore()
	registrar := &mockSkillRegistrar{existing: make(map[string]bool)}
	uc := NewSkillEvolutionUsecase(repo, repo, nil, nil, nil, registrar, loggateway.NewNoop())

	repo.seed(SkillProposal{
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
	repo := newMockUnifiedEvolutionStore()
	registrar := &mockSkillRegistrar{
		existing: map[string]bool{"a1:my-skill": true},
	}
	uc := NewSkillEvolutionUsecase(repo, repo, nil, nil, nil, registrar, loggateway.NewNoop())

	repo.seed(SkillProposal{
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
	repo := newMockUnifiedEvolutionStore()
	uc := NewSkillEvolutionUsecase(repo, repo, nil, nil, nil, nil, loggateway.NewNoop())

	repo.seed(SkillProposal{
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
	repo := newMockUnifiedEvolutionStore()
	uc := NewSkillEvolutionUsecase(repo, repo, nil, nil, nil, nil, loggateway.NewNoop())

	repo.seed(SkillProposal{
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
	uc := NewSkillEvolutionUsecase(newMockUnifiedEvolutionStore(), nil, nil, nil, nil, nil, loggateway.NewNoop())

	_, err := uc.GetProposal(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for empty id")
	}
}

func TestSkillEvolutionUsecase_ListProposals(t *testing.T) {
	repo := newMockUnifiedEvolutionStore()
	uc := NewSkillEvolutionUsecase(repo, repo, nil, nil, nil, nil, loggateway.NewNoop())

	repo.seed(SkillProposal{ID: "p1", AgentID: "a1", Status: SkillProposalStatusPending})
	repo.seed(SkillProposal{ID: "p2", AgentID: "a1", Status: SkillProposalStatusApproved})
	repo.seed(SkillProposal{ID: "p3", AgentID: "a2", Status: SkillProposalStatusPending})

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
	repo := newMockUnifiedEvolutionStore()
	uc := NewSkillEvolutionUsecase(repo, repo, nil, nil, nil, nil, loggateway.NewNoop())

	repo.seed(SkillProposal{ID: "p1", AgentID: "a1", Status: SkillProposalStatusPending})
	repo.seed(SkillProposal{ID: "p2", AgentID: "a1", Status: SkillProposalStatusApproved})
	repo.seed(SkillProposal{ID: "p3", AgentID: "a2", Status: SkillProposalStatusPending})

	all, err := uc.ListProposals(context.Background(), "", "", 0, 0)
	if err != nil {
		t.Fatalf("ListProposals with empty agentID: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("expected 3 proposals (all agents), got %d", len(all))
	}
}

func TestSkillEvolutionUsecase_DetectAndPropose_NoCreator(t *testing.T) {
	uc := NewSkillEvolutionUsecase(newMockUnifiedEvolutionStore(), nil, nil, &stubAgentRepo{agent: Agent{ID: "a1"}}, nil, nil, loggateway.NewNoop())

	proposals, err := uc.DetectAndPropose(context.Background(), "a1")
	if err != nil {
		t.Fatalf("DetectAndPropose: %v", err)
	}
	if len(proposals) != 0 {
		t.Errorf("expected 0 proposals without creator, got %d", len(proposals))
	}
}

func TestSkillEvolutionUsecase_DetectAndPropose_WithPatterns(t *testing.T) {
	repo := newMockUnifiedEvolutionStore()
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
	uc := NewSkillEvolutionUsecase(repo, repo, patterns, &stubAgentRepo{agent: Agent{ID: "a1"}}, creator, nil, loggateway.NewNoop())

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
	repo := newMockUnifiedEvolutionStore()
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
	uc := NewSkillEvolutionUsecase(repo, repo, patterns, &stubAgentRepo{agent: Agent{ID: "a1"}}, creator, nil, loggateway.NewNoop())

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
	repo := newMockUnifiedEvolutionStore()
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
	uc := NewSkillEvolutionUsecase(repo, repo, patterns, &stubAgentRepo{agent: Agent{ID: "a1"}}, creator, nil, loggateway.NewNoop())

	proposals, err := uc.DetectAndPropose(context.Background(), "a1")
	if err != nil {
		t.Fatalf("DetectAndPropose: %v", err)
	}
	if len(proposals) != 0 {
		t.Errorf("expected 0 proposals for low confidence, got %d", len(proposals))
	}
}

func TestSkillEvolutionUsecase_CreateProposal(t *testing.T) {
	repo := newMockUnifiedEvolutionStore()
	uc := NewSkillEvolutionUsecase(repo, repo, nil, nil, nil, nil, loggateway.NewNoop())

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

// TestSkillEvolutionUsecase_DetectAndPropose_NonExistentAgent verifies that
// DetectAndPropose rejects an agentID that does not correspond to an existing
// agent. This is the REL-2 fix: previously only requireNonEmpty was called.
func TestSkillEvolutionUsecase_DetectAndPropose_NonExistentAgent(t *testing.T) {
	repo := newMockUnifiedEvolutionStore()
	patterns := &mockPatternReader{
		patterns: []Pattern{
			{
				ID:          "pat1",
				AgentID:     "ghost",
				Kind:        string(ObservationKindToolCall),
				Description: "web_search(query)",
				Confidence:  0.8,
				Status:      PatternStatusDetected,
				DetectedAt:  time.Now().UTC(),
			},
		},
	}
	creator := &mockSkillAutoCreator{name: "web-search", content: "body"}
	// stubAgentRepo with a different agent ID → GetAgentByID returns ErrNotFound
	agents := &stubAgentRepo{agent: Agent{ID: "a1"}}
	uc := NewSkillEvolutionUsecase(repo, repo, patterns, agents, creator, nil, loggateway.NewNoop())

	proposals, err := uc.DetectAndPropose(context.Background(), "ghost")
	if err == nil {
		t.Fatal("expected error for non-existent agent, got nil")
	}
	if !isAPIErrorCode(err, apierror.CodeBadRequest) {
		t.Errorf("expected BadRequest for non-existent agent, got %v", err)
	}
	if len(proposals) != 0 {
		t.Errorf("expected 0 proposals, got %d", len(proposals))
	}
}

// TestSkillEvolutionUsecase_DetectAndPropose_NilAgentsFailClosed verifies that
// when the AgentRepository dependency is nil (misconfiguration), DetectAndPropose
// fails closed instead of silently proceeding. This is the REL-2 fix.
func TestSkillEvolutionUsecase_DetectAndPropose_NilAgentsFailClosed(t *testing.T) {
	repo := newMockUnifiedEvolutionStore()
	creator := &mockSkillAutoCreator{name: "web-search", content: "body"}
	uc := NewSkillEvolutionUsecase(repo, repo, nil, nil, creator, nil, loggateway.NewNoop())

	_, err := uc.DetectAndPropose(context.Background(), "a1")
	if err == nil {
		t.Fatal("expected error when agents dependency is nil, got nil")
	}
	if !isAPIErrorCode(err, apierror.CodeInternal) {
		t.Errorf("expected Internal when agents is nil, got %v", err)
	}
}
