package biz

import (
	"context"
	"fmt"
	"testing"
	"time"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// mockPatternRW mocks PatternReadWriter for pattern-status and dedup tests.
type mockPatternRW struct {
	byID       map[string]Pattern
	listResult []Pattern
	// listStatusFilter records the status argument of the last ListByAgent
	// call, letting dedup tests assert the usecase queries across all
	// statuses (""), not just "detected".
	listStatusFilter string
	created          []Pattern
	updateCalls      []PatternStatus
}

func newMockPatternRW(initial ...Pattern) *mockPatternRW {
	m := &mockPatternRW{byID: make(map[string]Pattern)}
	for _, p := range initial {
		m.byID[p.ID] = p
	}
	return m
}

func (m *mockPatternRW) ListByAgent(_ context.Context, _ string, status string) ([]Pattern, error) {
	m.listStatusFilter = status
	if m.listResult != nil {
		return m.listResult, nil
	}
	out := make([]Pattern, 0, len(m.byID))
	for _, p := range m.byID {
		if status == "" || string(p.Status) == status {
			out = append(out, p)
		}
	}
	return out, nil
}
func (m *mockPatternRW) GetByID(_ context.Context, id string) (Pattern, error) {
	p, ok := m.byID[id]
	if !ok {
		return Pattern{}, apierror.NotFound("LEARNING", "pattern not found")
	}
	return p, nil
}
func (m *mockPatternRW) Create(_ context.Context, p Pattern) (Pattern, error) {
	m.byID[p.ID] = p
	m.created = append(m.created, p)
	return p, nil
}
func (m *mockPatternRW) UpdateStatus(_ context.Context, id string, status PatternStatus) (Pattern, error) {
	m.updateCalls = append(m.updateCalls, status)
	p, ok := m.byID[id]
	if !ok {
		return Pattern{}, apierror.NotFound("LEARNING", "pattern not found")
	}
	p.Status = status
	m.byID[id] = p
	return p, nil
}

// mockObsRW mocks ObservationReadWriter for DetectPatterns dedup tests.
type mockObsRW struct {
	list []Observation
}

func (m *mockObsRW) ListByAgent(_ context.Context, _ string, _ time.Time) ([]Observation, error) {
	return m.list, nil
}
func (m *mockObsRW) CountByAgent(_ context.Context, _ string, _ time.Time) (int64, error) {
	return int64(len(m.list)), nil
}
func (m *mockObsRW) Create(_ context.Context, o Observation) (Observation, error) { return o, nil }
func (m *mockObsRW) BatchCreate(_ context.Context, _ []Observation) error         { return nil }

// TestApproveProposal_ApproveAndRegister covers the "审批即注册" flow:
// a validated proposal must end up applied (not stranded at approved),
// because the UI approval dialog promises registration into the KB.
func TestApproveProposal_ApproveAndRegister(t *testing.T) {
	proposal := KnowledgeProposal{
		ID:      "prop-approve-register",
		AgentID: "agent-1",
		Kind:    "prompt",
		Title:   "Approve and register",
		Status:  ProposalStatusValidated,
	}
	propRW := newMockProposalRW(proposal)
	uc := &LearningLoopUsecase{proposals: propRW, lg: loggateway.NewNoop()}

	got, err := uc.ApproveProposal(context.Background(), proposal.ID, "user:1")
	if err != nil {
		t.Fatalf("ApproveProposal: %v", err)
	}
	if got.Status != ProposalStatusApplied {
		t.Fatalf("expected status=%s, got=%s", ProposalStatusApplied, got.Status)
	}
	// Expect two CAS transitions recorded: validated→approved, then
	// approved→applied (RegisterKnowledge).
	if len(propRW.updCalls) != 2 {
		t.Fatalf("expected 2 status updates, got %d: %+v", len(propRW.updCalls), propRW.updCalls)
	}
	if propRW.updCalls[0].Status != ProposalStatusApproved || propRW.updCalls[1].Status != ProposalStatusApplied {
		t.Errorf("unexpected transition chain: %+v", propRW.updCalls)
	}
}

// TestApproveProposal_ApprovedRetriesRegister covers the idempotent retry
// path: a proposal stranded at approved (e.g. earlier registration failure)
// must be re-registered by calling ApproveProposal again.
func TestApproveProposal_ApprovedRetriesRegister(t *testing.T) {
	proposal := KnowledgeProposal{
		ID:      "prop-approved-retry",
		AgentID: "agent-1",
		Kind:    "prompt",
		Title:   "Approved retry",
		Status:  ProposalStatusApproved,
	}
	propRW := newMockProposalRW(proposal)
	uc := &LearningLoopUsecase{proposals: propRW, lg: loggateway.NewNoop()}

	got, err := uc.ApproveProposal(context.Background(), proposal.ID, "user:1")
	if err != nil {
		t.Fatalf("ApproveProposal: %v", err)
	}
	if got.Status != ProposalStatusApplied {
		t.Fatalf("expected status=%s, got=%s", ProposalStatusApplied, got.Status)
	}
}

func TestApproveProposal_RejectsTerminalStatus(t *testing.T) {
	for _, st := range []ProposalStatus{ProposalStatusDraft, ProposalStatusRejected, ProposalStatusApplied, ProposalStatusConflict} {
		proposal := KnowledgeProposal{ID: "prop-" + string(st), AgentID: "a", Status: st}
		propRW := newMockProposalRW(proposal)
		uc := &LearningLoopUsecase{proposals: propRW, lg: loggateway.NewNoop()}
		_, err := uc.ApproveProposal(context.Background(), proposal.ID, "user:1")
		if err == nil {
			t.Fatalf("status=%s: expected error, got nil", st)
		}
		ae, ok := apierror.From(err)
		if !ok || ae.Code != apierror.CodeBadRequest {
			t.Fatalf("status=%s: expected BadRequest, got: %v", st, err)
		}
	}
}

func TestApplyProposal_ValidatedToApplied(t *testing.T) {
	proposal := KnowledgeProposal{
		ID:      "prop-apply",
		AgentID: "agent-1",
		Kind:    "prompt",
		Title:   "Apply",
		Status:  ProposalStatusValidated,
	}
	propRW := newMockProposalRW(proposal)
	uc := &LearningLoopUsecase{proposals: propRW, lg: loggateway.NewNoop()}

	got, err := uc.ApplyProposal(context.Background(), proposal.ID, "user:2")
	if err != nil {
		t.Fatalf("ApplyProposal: %v", err)
	}
	if got.Status != ProposalStatusApplied {
		t.Fatalf("expected status=%s, got=%s", ProposalStatusApplied, got.Status)
	}
}

func TestUpdatePatternStatus_DetectedToConfirmedAndDismissed(t *testing.T) {
	for _, target := range []PatternStatus{PatternStatusConfirmed, PatternStatusDismissed} {
		p := Pattern{ID: "pat-" + string(target), AgentID: "a", Status: PatternStatusDetected}
		patRW := newMockPatternRW(p)
		uc := &LearningLoopUsecase{patterns: patRW, lg: loggateway.NewNoop()}

		got, err := uc.UpdatePatternStatus(context.Background(), p.ID, target)
		if err != nil {
			t.Fatalf("UpdatePatternStatus(%s): %v", target, err)
		}
		if got.Status != target {
			t.Fatalf("expected status=%s, got=%s", target, got.Status)
		}
	}
}

func TestUpdatePatternStatus_RejectsNonDetected(t *testing.T) {
	p := Pattern{ID: "pat-confirmed", AgentID: "a", Status: PatternStatusConfirmed}
	patRW := newMockPatternRW(p)
	uc := &LearningLoopUsecase{patterns: patRW, lg: loggateway.NewNoop()}

	_, err := uc.UpdatePatternStatus(context.Background(), p.ID, PatternStatusDismissed)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if ae, ok := apierror.From(err); !ok || ae.Code != apierror.CodeBadRequest {
		t.Fatalf("expected BadRequest, got: %v", err)
	}
}

func TestUpdatePatternStatus_RejectsInvalidStatus(t *testing.T) {
	p := Pattern{ID: "pat-detected", AgentID: "a", Status: PatternStatusDetected}
	patRW := newMockPatternRW(p)
	uc := &LearningLoopUsecase{patterns: patRW, lg: loggateway.NewNoop()}

	for _, bad := range []PatternStatus{PatternStatusDetected, "bogus"} {
		if _, err := uc.UpdatePatternStatus(context.Background(), p.ID, bad); err == nil {
			t.Fatalf("status=%s: expected error, got nil", bad)
		}
	}
}

// TestDetectPatterns_DedupCoversAllStatuses is a regression test for the
// finding "忽略模式后重复检出": dedup previously only scanned detected
// patterns, so a dismissed pattern re-appeared on the next RunLoop.
func TestDetectPatterns_DedupCoversAllStatuses(t *testing.T) {
	agentID := "agent-dedup"
	// 3 tool_call observations of the same tool → one bucket with a stable
	// description ("高频工具调用模式: gns3_exec(3)").
	obs := make([]Observation, 0, 3)
	for i := 0; i < 3; i++ {
		obs = append(obs, Observation{
			ID:         fmt.Sprintf("obs-%d", i),
			AgentID:    agentID,
			Kind:       ObservationKindToolCall,
			Metadata:   `{"tool_name":"gns3_exec"}`,
			ObservedAt: time.Now().UTC(),
		})
	}
	dismissed := Pattern{
		ID:          "pat-dismissed",
		AgentID:     agentID,
		Kind:        string(ObservationKindToolCall),
		Description: describeBucket(string(ObservationKindToolCall), obs),
		Status:      PatternStatusDismissed,
	}
	patRW := newMockPatternRW(dismissed)
	uc := &LearningLoopUsecase{
		obs:      &mockObsRW{list: obs},
		patterns: patRW,
		lg:       loggateway.NewNoop(),
	}

	created, err := uc.DetectPatterns(context.Background(), agentID)
	if err != nil {
		t.Fatalf("DetectPatterns: %v", err)
	}
	if len(created) != 0 {
		t.Fatalf("expected no new patterns (dismissed must suppress re-detection), got %d: %+v", len(created), created)
	}
	if patRW.listStatusFilter != "" {
		t.Fatalf("expected dedup query across all statuses (\"\"), got %q", patRW.listStatusFilter)
	}
}
