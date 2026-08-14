package biz

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// mockProposalRW mocks ProposalReadWriter for RegisterKnowledge tests.
type mockProposalRW struct {
	mu       sync.Mutex
	byID     map[string]KnowledgeProposal
	updCalls []updStatusCall
	updErr   error
	// strictCAS, when true (default), makes UpdateStatusCAS enforce the
	// expected-statuses predicate against the stored proposal. When false,
	// CAS always succeeds (legacy behavior — only for back-compat tests).
	strictCAS bool
	// onCAS, if non-nil, is invoked inside UpdateStatusCAS BEFORE evaluating
	// the CAS predicate. Tests use this to simulate a concurrent transition
	// by mutating the stored proposal — exactly the TOCTOU window that the
	// usecase must defend against.
	onCAS func(stored *KnowledgeProposal)
	// listResult, if non-nil, is returned by ListByAgent (overrides the
	// default nil). Used by ValidateProposal's conflict-detection branch
	// to simulate pre-existing approved proposals.
	listResult []KnowledgeProposal
}

type updStatusCall struct {
	ID         string
	Status     ProposalStatus
	ApprovedBy string
}

func (m *mockProposalRW) ListByAgent(_ context.Context, _ string, _ string) ([]KnowledgeProposal, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.listResult != nil {
		return m.listResult, nil
	}
	return nil, nil
}
func (m *mockProposalRW) GetByID(_ context.Context, id string) (KnowledgeProposal, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.byID[id]
	if !ok {
		return KnowledgeProposal{}, apierror.NotFound("LEARNING", "proposal not found")
	}
	return p, nil
}
func (m *mockProposalRW) Create(_ context.Context, p KnowledgeProposal) (KnowledgeProposal, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	// Persist to byID so read-after-write flows (e.g. RunLoop: GenerateProposals
	// → ValidateProposal via GetByID) behave like the real DB-backed repo.
	m.byID[p.ID] = p
	return p, nil
}
func (m *mockProposalRW) UpdateStatus(_ context.Context, id string, status ProposalStatus, approvedBy string) (KnowledgeProposal, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.updCalls = append(m.updCalls, updStatusCall{ID: id, Status: status, ApprovedBy: approvedBy})
	if m.updErr != nil {
		return KnowledgeProposal{}, m.updErr
	}
	p := m.byID[id]
	p.Status = status
	m.byID[id] = p
	return p, nil
}

// UpdateStatusCAS implements the CAS variant. When strictCAS is true (default),
// the stored status must be in expectedStatuses for the transition to succeed.
// The onCAS hook (if set) fires before the CAS check, simulating a concurrent
// transition that mutates the stored state.
func (m *mockProposalRW) UpdateStatusCAS(_ context.Context, id string, expectedStatuses []ProposalStatus, newStatus ProposalStatus, approvedBy string) (KnowledgeProposal, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.updCalls = append(m.updCalls, updStatusCall{ID: id, Status: newStatus, ApprovedBy: approvedBy})
	if m.updErr != nil {
		return KnowledgeProposal{}, false, m.updErr
	}
	p, ok := m.byID[id]
	if !ok {
		return KnowledgeProposal{}, false, nil
	}
	// Fire the TOCTOU simulation hook before evaluating the CAS predicate.
	if m.onCAS != nil {
		m.onCAS(&p)
		m.byID[id] = p
	}
	if m.strictCAS {
		matched := false
		for _, e := range expectedStatuses {
			if p.Status == e {
				matched = true
				break
			}
		}
		if !matched {
			return KnowledgeProposal{}, false, nil
		}
	}
	p.Status = newStatus
	m.byID[id] = p
	return p, true, nil
}

// newMockProposalRW returns a mock with strict CAS enforcement enabled by
// default. Tests that need legacy blind-update behavior can flip strictCAS
// to false; tests that need to simulate a concurrent transition can set
// onCAS to mutate the stored proposal.
func newMockProposalRW(initial ...KnowledgeProposal) *mockProposalRW {
	m := &mockProposalRW{
		byID:      make(map[string]KnowledgeProposal),
		strictCAS: true,
	}
	for _, p := range initial {
		m.byID[p.ID] = p
	}
	return m
}

// mockUnifiedWriter mocks UnifiedEvolutionWriter for RegisterKnowledge tests.
type mockUnifiedWriter struct {
	createErr  error
	createCall int
}

func (m *mockUnifiedWriter) Create(_ context.Context, _ UnifiedEvolutionSuggestion) error {
	m.createCall++
	return m.createErr
}
func (m *mockUnifiedWriter) UpdateStatus(context.Context, string, string, string, string) error {
	return nil
}
func (m *mockUnifiedWriter) UpdateStatusCAS(context.Context, string, []string, string, string, string) (bool, error) {
	return false, nil
}
func (m *mockUnifiedWriter) UpdateDraftBody(context.Context, string, string) error { return nil }
func (m *mockUnifiedWriter) UpdateLifecycleStatus(context.Context, string, string) error {
	return nil
}
func (m *mockUnifiedWriter) UpdateSandboxResult(context.Context, string, bool, json.RawMessage) error {
	return nil
}
func (m *mockUnifiedWriter) UpdateMetadataKey(context.Context, string, string, string) error {
	return nil
}
func (m *mockUnifiedWriter) ExpireOlderThan(context.Context, time.Time) (int, error) {
	return 0, nil
}

// TestRegisterKnowledge_CreateSuggestionFailureDoesNotMarkApplied is a
// regression test for the audit finding "创建统一演化建议失败仍将 Proposal
// 标为 applied" (Domain 7 Claim 5).
//
// Previously, RegisterKnowledge silently swallowed CreateSuggestion errors
// and marked the proposal as "applied" regardless. Now, non-conflict errors
// must propagate and prevent the status update.
func TestRegisterKnowledge_CreateSuggestionFailureDoesNotMarkApplied(t *testing.T) {
	// Set up a proposal in "validated" state (eligible for registration).
	proposal := KnowledgeProposal{
		ID:      "prop-1",
		AgentID: "agent-1",
		Kind:    "prompt",
		Title:   "Improve greeting",
		Content: "Add warm greeting",
		Status:  ProposalStatusValidated,
	}
	propRW := newMockProposalRW(proposal)

	// Orchestrator writer that simulates a DB failure (non-conflict).
	writer := &mockUnifiedWriter{createErr: errors.New("db connection lost")}
	orch := NewSkillEvolutionOrchestrator(nil, writer, loggateway.NewNoop())

	uc := &LearningLoopUsecase{
		proposals:    propRW,
		orchestrator: orch,
		lg:           loggateway.NewNoop(),
	}

	_, err := uc.RegisterKnowledge(context.Background(), "prop-1", "tester")
	if err == nil {
		t.Fatal("expected error when CreateSuggestion fails, got nil")
	}

	// Proposal must NOT be marked as applied.
	for _, call := range propRW.updCalls {
		if call.Status == ProposalStatusApplied {
			t.Fatalf("proposal must NOT be marked applied when suggestion creation fails; got UpdateStatus call with status=%s", call.Status)
		}
	}
}

// TestRegisterKnowledge_CreateSuggestionSuccessMarksApplied covers the happy
// path: when CreateSuggestion succeeds, the proposal should be marked applied.
func TestRegisterKnowledge_CreateSuggestionSuccessMarksApplied(t *testing.T) {
	proposal := KnowledgeProposal{
		ID:      "prop-2",
		AgentID: "agent-2",
		Kind:    "prompt",
		Title:   "Refine closing",
		Content: "Add polite closing",
		Status:  ProposalStatusValidated,
	}
	propRW := newMockProposalRW(proposal)
	writer := &mockUnifiedWriter{createErr: nil}
	orch := NewSkillEvolutionOrchestrator(nil, writer, loggateway.NewNoop())

	uc := &LearningLoopUsecase{
		proposals:    propRW,
		orchestrator: orch,
		lg:           loggateway.NewNoop(),
	}

	applied, err := uc.RegisterKnowledge(context.Background(), "prop-2", "tester")
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	if applied.Status != ProposalStatusApplied {
		t.Errorf("expected status=%s, got=%s", ProposalStatusApplied, applied.Status)
	}
	if writer.createCall != 1 {
		t.Errorf("expected CreateSuggestion to be called once, got %d", writer.createCall)
	}
}

// ===== Domain 7 Claim 4 TOCTOU regression tests =====
//
// These tests verify that the four state-transition usecase methods
// (ValidateProposal, ApproveProposal, RejectProposal, RegisterKnowledge)
// defend against TOCTOU races by using UpdateStatusCAS. The onCAS hook
// on mockProposalRW simulates a concurrent transition that mutates the
// stored proposal between the usecase's GetByID read and its CAS write.
//
// Without CAS, the usecase would observe status=S1 in GetByID, pass the
// local `if p.Status != X` check, then blindly overwrite the concurrent
// transition. With CAS, the usecase must return a Conflict error and
// leave the concurrently-transitioned status intact.

// TestValidateProposal_CASPreventsConcurrentTransition simulates a
// concurrent Approve call that transitions the proposal from Draft to
// Approved between the usecase's GetByID read and its UpdateStatusCAS
// write. The CAS must reject the transition (return Conflict) and the
// stored status must remain Approved (not Validated).
func TestValidateProposal_CASPreventsConcurrentTransition(t *testing.T) {
	proposal := KnowledgeProposal{
		ID:      "prop-validate-race",
		AgentID: "agent-race",
		Kind:    "prompt",
		Title:   "Race test",
		Status:  ProposalStatusDraft,
	}
	propRW := newMockProposalRW(proposal)
	// Simulate a concurrent ApproveProposal winning the race: at the
	// moment UpdateStatusCAS is called, the stored status flips to
	// Approved, which is not in the expected [Draft] set.
	propRW.onCAS = func(stored *KnowledgeProposal) {
		stored.Status = ProposalStatusApproved
	}

	uc := &LearningLoopUsecase{proposals: propRW, lg: loggateway.NewNoop()}
	_, err := uc.ValidateProposal(context.Background(), "prop-validate-race")
	if err == nil {
		t.Fatal("expected Conflict error when CAS detects concurrent transition, got nil")
	}
	ae, ok := apierror.From(err)
	if !ok || ae.Code != apierror.CodeConflict {
		t.Fatalf("expected Conflict error, got: %v", err)
	}
	// Stored status must remain Approved (the concurrent transition's result).
	current, _ := propRW.GetByID(context.Background(), "prop-validate-race")
	if current.Status != ProposalStatusApproved {
		t.Errorf("expected stored status=%s (concurrent transition preserved), got=%s", ProposalStatusApproved, current.Status)
	}
}

// TestValidateProposal_ConflictBranch_CASPreventsConcurrentTransition
// covers the conflict-detection branch of ValidateProposal: when a
// same-kind/same-title approved proposal exists, the usecase attempts
// to transition the current proposal to Conflict status. A concurrent
// transition that mutates the stored status away from Draft must cause
// the CAS to fail and return Conflict.
func TestValidateProposal_ConflictBranch_CASPreventsConcurrentTransition(t *testing.T) {
	proposal := KnowledgeProposal{
		ID:      "prop-conflict-race",
		AgentID: "agent-race",
		Kind:    "prompt",
		Title:   "Dup title",
		Status:  ProposalStatusDraft,
	}
	// Pre-existing approved proposal with same kind/title triggers the
	// conflict branch in ValidateProposal.
	dup := KnowledgeProposal{
		ID:      "prop-existing-approved",
		AgentID: "agent-race",
		Kind:    "prompt",
		Title:   "Dup title",
		Status:  ProposalStatusApproved,
	}
	propRW := newMockProposalRW(proposal)
	propRW.listResult = []KnowledgeProposal{dup}
	propRW.onCAS = func(stored *KnowledgeProposal) {
		stored.Status = ProposalStatusValidated // concurrent Validate won
	}

	uc := &LearningLoopUsecase{proposals: propRW, lg: loggateway.NewNoop()}
	_, err := uc.ValidateProposal(context.Background(), "prop-conflict-race")
	if err == nil {
		t.Fatal("expected Conflict error when CAS detects concurrent transition in conflict branch, got nil")
	}
	ae, ok := apierror.From(err)
	if !ok || ae.Code != apierror.CodeConflict {
		t.Fatalf("expected Conflict error, got: %v", err)
	}
	current, _ := propRW.GetByID(context.Background(), "prop-conflict-race")
	if current.Status != ProposalStatusValidated {
		t.Errorf("expected stored status=%s (concurrent Validate preserved), got=%s", ProposalStatusValidated, current.Status)
	}
}

// TestApproveProposal_CASPreventsConcurrentTransition simulates a
// concurrent RegisterKnowledge that transitions the proposal from
// Validated to Applied between the usecase's GetByID read and its
// UpdateStatusCAS write.
func TestApproveProposal_CASPreventsConcurrentTransition(t *testing.T) {
	proposal := KnowledgeProposal{
		ID:      "prop-approve-race",
		AgentID: "agent-race",
		Kind:    "prompt",
		Title:   "Approve race",
		Status:  ProposalStatusValidated,
	}
	propRW := newMockProposalRW(proposal)
	propRW.onCAS = func(stored *KnowledgeProposal) {
		stored.Status = ProposalStatusApplied // concurrent Register won
	}

	uc := &LearningLoopUsecase{proposals: propRW, lg: loggateway.NewNoop()}
	_, err := uc.ApproveProposal(context.Background(), "prop-approve-race", "tester")
	if err == nil {
		t.Fatal("expected Conflict error, got nil")
	}
	ae, ok := apierror.From(err)
	if !ok || ae.Code != apierror.CodeConflict {
		t.Fatalf("expected Conflict error, got: %v", err)
	}
	current, _ := propRW.GetByID(context.Background(), "prop-approve-race")
	if current.Status != ProposalStatusApplied {
		t.Errorf("expected stored status=%s, got=%s", ProposalStatusApplied, current.Status)
	}
}

// TestRejectProposal_CASPreventsConcurrentTransition simulates a
// concurrent Approve call that transitions the proposal from Draft to
// Approved between the usecase's GetByID read and its UpdateStatusCAS
// write.
func TestRejectProposal_CASPreventsConcurrentTransition(t *testing.T) {
	proposal := KnowledgeProposal{
		ID:      "prop-reject-race",
		AgentID: "agent-race",
		Kind:    "prompt",
		Title:   "Reject race",
		Status:  ProposalStatusDraft,
	}
	propRW := newMockProposalRW(proposal)
	propRW.onCAS = func(stored *KnowledgeProposal) {
		stored.Status = ProposalStatusApproved // concurrent Approve won
	}

	uc := &LearningLoopUsecase{proposals: propRW, lg: loggateway.NewNoop()}
	_, err := uc.RejectProposal(context.Background(), "prop-reject-race")
	if err == nil {
		t.Fatal("expected Conflict error, got nil")
	}
	ae, ok := apierror.From(err)
	if !ok || ae.Code != apierror.CodeConflict {
		t.Fatalf("expected Conflict error, got: %v", err)
	}
	current, _ := propRW.GetByID(context.Background(), "prop-reject-race")
	if current.Status != ProposalStatusApproved {
		t.Errorf("expected stored status=%s, got=%s", ProposalStatusApproved, current.Status)
	}
}

// TestRegisterKnowledge_CASPreventsConcurrentTransition simulates a
// concurrent Reject call that transitions the proposal from Validated
// to Rejected between the usecase's GetByID read and its
// UpdateStatusCAS write. The orchestrator's suggestion is created
// before the CAS fires (this is a known limitation — the pending
// suggestion is harmless and will be deduped on retry).
func TestRegisterKnowledge_CASPreventsConcurrentTransition(t *testing.T) {
	proposal := KnowledgeProposal{
		ID:      "prop-register-race",
		AgentID: "agent-race",
		Kind:    "prompt",
		Title:   "Register race",
		Status:  ProposalStatusValidated,
	}
	propRW := newMockProposalRW(proposal)
	propRW.onCAS = func(stored *KnowledgeProposal) {
		stored.Status = ProposalStatusRejected // concurrent Reject won
	}
	writer := &mockUnifiedWriter{createErr: nil}
	orch := NewSkillEvolutionOrchestrator(nil, writer, loggateway.NewNoop())

	uc := &LearningLoopUsecase{
		proposals:    propRW,
		orchestrator: orch,
		lg:           loggateway.NewNoop(),
	}
	_, err := uc.RegisterKnowledge(context.Background(), "prop-register-race", "tester")
	if err == nil {
		t.Fatal("expected Conflict error, got nil")
	}
	ae, ok := apierror.From(err)
	if !ok || ae.Code != apierror.CodeConflict {
		t.Fatalf("expected Conflict error, got: %v", err)
	}
	current, _ := propRW.GetByID(context.Background(), "prop-register-race")
	if current.Status != ProposalStatusRejected {
		t.Errorf("expected stored status=%s, got=%s", ProposalStatusRejected, current.Status)
	}
	// The orchestrator's suggestion was created before the CAS fired —
	// this is acceptable because the suggestion is in "pending" status
	// and will be deduped (Conflict) on the next retry.
	if writer.createCall != 1 {
		t.Errorf("expected orchestrator.Create to be called once before CAS, got %d", writer.createCall)
	}
}

// TestValidateProposal_CASSucceedsWhenStatusMatches is a positive test
// verifying that CAS succeeds (returns the transitioned proposal) when
// no concurrent modification happens.
func TestValidateProposal_CASSucceedsWhenStatusMatches(t *testing.T) {
	proposal := KnowledgeProposal{
		ID:      "prop-validate-ok",
		AgentID: "agent-ok",
		Kind:    "prompt",
		Title:   "Happy path",
		Status:  ProposalStatusDraft,
	}
	propRW := newMockProposalRW(proposal)

	uc := &LearningLoopUsecase{proposals: propRW, lg: loggateway.NewNoop()}
	out, err := uc.ValidateProposal(context.Background(), "prop-validate-ok")
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if out.Status != ProposalStatusValidated {
		t.Errorf("expected status=%s, got=%s", ProposalStatusValidated, out.Status)
	}
}
