package biz

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

type cascadeGraphStoreMock struct {
	inserted []CascadeProposalInsert
	nbJSON   []byte
}

func (m *cascadeGraphStoreMock) InsertCascadeProposal(_ context.Context, in CascadeProposalInsert) ([]byte, error) {
	m.inserted = append(m.inserted, in)
	return json.Marshal(map[string]any{
		"id": "cp-1", "agent_id": in.AgentID, "status": "pending",
		"trigger_entity_id": in.TriggerEntityID, "old_value": in.OldValue, "new_value": in.NewValue,
		"affected_json": in.AffectedJSON,
	})
}

func (m *cascadeGraphStoreMock) ListCascadeProposalRows(context.Context, string, string, int32) ([][]byte, error) {
	return nil, nil
}

func (m *cascadeGraphStoreMock) GetCascadeProposalRow(context.Context, string) ([]byte, error) {
	return nil, nil
}

func (m *cascadeGraphStoreMock) UpdateCascadeProposalStatus(context.Context, string, string, string, string) ([]byte, error) {
	return nil, nil
}

func (m *cascadeGraphStoreMock) CompareAndSwapProposalStatus(_ context.Context, _ string, _ []string, toStatus, _, _ string) ([]byte, bool, error) {
	b, _ := json.Marshal(map[string]any{"id": "cp-1", "status": toStatus})
	return b, true, nil
}

func (m *cascadeGraphStoreMock) NeighborhoodJSON(context.Context, string, int32, int32, string) ([]byte, error) {
	if len(m.nbJSON) == 0 {
		return []byte(`{"entities":[{"id":"e2","name":"Bob","entity_type":"person"}],"relations":[{"target_id":"e2","relation_type":"knows_as"}]}`), nil
	}
	return m.nbJSON, nil
}

func (m *cascadeGraphStoreMock) GetEntityRow(context.Context, string) ([]byte, error) {
	return nil, nil
}

func (m *cascadeGraphStoreMock) ReplaceNameInAgentFacts(context.Context, string, string, string) ([][]byte, int, error) {
	return nil, 0, nil
}

func (m *cascadeGraphStoreMock) InitCascadeSagaSteps(_ context.Context, _ string, _ []CascadeSagaStep) error {
	return nil
}

func (m *cascadeGraphStoreMock) GetCascadeSagaSteps(_ context.Context, _ string) ([]CascadeSagaStep, error) {
	return nil, nil
}

func (m *cascadeGraphStoreMock) UpdateSagaStepState(_ context.Context, _ string, _, _ string) error {
	return nil
}

func (m *cascadeGraphStoreMock) UpdateSagaStepResult(_ context.Context, _ string, _ string) error {
	return nil
}

func (m *cascadeGraphStoreMock) HasCascadeSaga(_ context.Context, _ string) (bool, error) {
	return false, nil
}

func (m *cascadeGraphStoreMock) SaveCascadeOriginalStatements(_ context.Context, _, _ string, _ []string) error {
	return nil
}

func (m *cascadeGraphStoreMock) RevertCascadeFactStatements(_ context.Context, _ string) (int, error) {
	return 0, nil
}

func (m *cascadeGraphStoreMock) ListCascadeFactDiffs(_ context.Context, _, _, _ string, _ int) ([]map[string]any, error) {
	return nil, nil
}

func (m *cascadeGraphStoreMock) MarkFactsIndexStaleByAgent(_ context.Context, _ string) (int64, error) {
	return 0, nil
}

func TestL4CascadeUsecase_ProposeNameConflict(t *testing.T) {
	store := &cascadeGraphStoreMock{}
	uc := NewL4CascadeUsecase(L4CascadeDeps{Proposals: store, Reader: store, Mutator: store, Saga: store, LG: loggateway.NewNoop()})
	if err := uc.ProposeNameConflict(context.Background(), "ag1", "ent1", "Alice", "Bob"); err != nil {
		t.Fatal(err)
	}
	if len(store.inserted) != 1 {
		t.Fatalf("expected 1 proposal, got %d", len(store.inserted))
	}
	p := store.inserted[0]
	if p.OldValue != "Alice" || p.NewValue != "Bob" || p.TriggerAttribute != "name" {
		t.Fatalf("unexpected insert: %+v", p)
	}
	if p.RiskLevel != "low" {
		t.Fatalf("expected low risk, got %q", p.RiskLevel)
	}
}

func TestL4CascadeUsecase_ProposeNameConflict_SkipSameName(t *testing.T) {
	store := &cascadeGraphStoreMock{}
	uc := NewL4CascadeUsecase(L4CascadeDeps{Proposals: store, Reader: store, Mutator: store, Saga: store, LG: loggateway.NewNoop()})
	if err := uc.ProposeNameConflict(context.Background(), "ag1", "ent1", "Alice", "alice"); err != nil {
		t.Fatal(err)
	}
	if len(store.inserted) != 0 {
		t.Fatal("expected no proposal for same name")
	}
}

func TestCascadeRiskLevel(t *testing.T) {
	if cascadeRiskLevel(0) != "low" || cascadeRiskLevel(2) != "medium" || cascadeRiskLevel(5) != "high" {
		t.Fatal("unexpected risk levels")
	}
}

type cascadeApproveStore struct {
	cascadeGraphStoreMock
	proposal []byte
}

func (m *cascadeApproveStore) GetCascadeProposalRow(_ context.Context, _ string) ([]byte, error) {
	return m.proposal, nil
}

func (m *cascadeApproveStore) GetEntityRow(_ context.Context, _ string) ([]byte, error) {
	return []byte(`{"id":"ent1","user_id":"u1","entity_type":"person","name":"Alice","name_normalized":"alice","description":"d","metadata_json":"{}"}`), nil
}

func (m *cascadeApproveStore) UpdateCascadeProposalStatus(_ context.Context, id, status, reviewer, note string) ([]byte, error) {
	return json.Marshal(map[string]any{"id": id, "status": status, "reviewed_by": reviewer, "review_note": note})
}

func (m *cascadeApproveStore) CompareAndSwapProposalStatus(_ context.Context, _ string, _ []string, toStatus, _, _ string) ([]byte, bool, error) {
	// Return the full proposal row (with all fields) so that Approve can
	// extract trigger_entity_id, new_value, etc. after CAS succeeds.
	b, _ := json.Marshal(map[string]any{
		"id":                "cp1",
		"agent_id":          "ag1",
		"status":            toStatus,
		"trigger_entity_id": "ent1",
		"old_value":         "Alice",
		"new_value":         "Bob",
		"affected_json":     `[{"entity_id":"n1","entity_name":"Neighbor","entity_type":"person","relation_type":"knows_as","hops":1}]`,
	})
	return b, true, nil
}

func (m *cascadeApproveStore) HasCascadeSaga(_ context.Context, _ string) (bool, error) {
	return false, nil
}

func (m *cascadeApproveStore) InitCascadeSagaSteps(_ context.Context, _ string, _ []CascadeSagaStep) error {
	return nil
}

func (m *cascadeApproveStore) GetCascadeSagaSteps(_ context.Context, proposalID string) ([]CascadeSagaStep, error) {
	return []CascadeSagaStep{
		{ID: "step-1", ProposalID: proposalID, StepIndex: 0, StepName: SagaStepUpsertEntity, State: "pending", IsCritical: true},
		{ID: "step-2", ProposalID: proposalID, StepIndex: 1, StepName: SagaStepTouchAffected, State: "pending", IsCritical: false},
		{ID: "step-3", ProposalID: proposalID, StepIndex: 2, StepName: SagaStepReplaceFacts, State: "pending", IsCritical: true},
		{ID: "step-4", ProposalID: proposalID, StepIndex: 3, StepName: SagaStepSyncIndex, State: "pending", IsCritical: false},
	}, nil
}

func (m *cascadeApproveStore) ReplaceNameInAgentFacts(_ context.Context, _, _, _ string) ([][]byte, int, error) {
	return nil, 0, nil
}

func (m *cascadeApproveStore) ListCascadeFactDiffs(_ context.Context, _, _, _ string, _ int) ([]map[string]any, error) {
	return nil, nil
}

func (m *cascadeApproveStore) SaveCascadeOriginalStatements(_ context.Context, _, _ string, _ []string) error {
	return nil
}

func (m *cascadeApproveStore) UpdateSagaStepState(_ context.Context, _ string, _, _ string) error {
	return nil
}

func (m *cascadeApproveStore) UpdateSagaStepResult(_ context.Context, _ string, _ string) error {
	return nil
}

func (m *cascadeApproveStore) RevertCascadeFactStatements(_ context.Context, _ string) (int, error) {
	return 0, nil
}

func (m *cascadeApproveStore) MarkFactsIndexStaleByAgent(_ context.Context, _ string) (int64, error) {
	return 0, nil
}

func TestL4CascadeUsecase_Approve(t *testing.T) {
	repo := &mockL4GraphRepo{}
	store := &cascadeApproveStore{
		proposal: []byte(`{
			"id":"cp1","agent_id":"ag1","status":"pending",
			"trigger_entity_id":"ent1","new_value":"Bob",
			"affected_json":"[{\"entity_id\":\"n1\",\"entity_name\":\"Neighbor\",\"entity_type\":\"person\",\"relation_type\":\"knows_as\",\"hops\":1}]"
		}`),
	}
	uc := NewL4CascadeUsecase(L4CascadeDeps{Proposals: store, Reader: store, Mutator: store, Saga: store, EntityWriter: repo, LG: loggateway.NewNoop()})
	raw, err := uc.Approve(context.Background(), "cp1", "reviewer-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) == 0 {
		t.Fatal("expected proposal row")
	}
	var applied bool
	for _, e := range repo.entities {
		if e.ID == "ent1" && e.Name == "Bob" {
			applied = true
		}
	}
	if !applied {
		t.Fatal("expected trigger entity renamed on approve")
	}
}

// cascadeCompensateStore tracks in-memory saga step state and forces
// ReplaceFacts to fail so Approve must auto-compensate earlier successes.
type cascadeCompensateStore struct {
	cascadeApproveStore
	steps         []CascadeSagaStep
	stepStates    map[string]string
	replaceErr    error
	statusHistory []string
}

func (m *cascadeCompensateStore) InitCascadeSagaSteps(_ context.Context, proposalID string, steps []CascadeSagaStep) error {
	m.steps = make([]CascadeSagaStep, len(steps))
	m.stepStates = make(map[string]string)
	for i, s := range steps {
		s.ID = fmt.Sprintf("step-%d", i+1)
		s.ProposalID = proposalID
		s.StepIndex = i
		s.State = "pending"
		m.steps[i] = s
		m.stepStates[s.ID] = "pending"
	}
	return nil
}

func (m *cascadeCompensateStore) GetCascadeSagaSteps(_ context.Context, _ string) ([]CascadeSagaStep, error) {
	out := make([]CascadeSagaStep, len(m.steps))
	copy(out, m.steps)
	for i := range out {
		if st, ok := m.stepStates[out[i].ID]; ok {
			out[i].State = st
		}
	}
	return out, nil
}

func (m *cascadeCompensateStore) UpdateSagaStepState(_ context.Context, stepID string, state, _ string) error {
	if m.stepStates == nil {
		m.stepStates = make(map[string]string)
	}
	m.stepStates[stepID] = state
	for i := range m.steps {
		if m.steps[i].ID == stepID {
			m.steps[i].State = state
		}
	}
	return nil
}

func (m *cascadeCompensateStore) ReplaceNameInAgentFacts(context.Context, string, string, string) ([][]byte, int, error) {
	if m.replaceErr != nil {
		return nil, 0, m.replaceErr
	}
	return nil, 0, nil
}

func (m *cascadeCompensateStore) UpdateCascadeProposalStatus(_ context.Context, id, status, reviewer, note string) ([]byte, error) {
	m.statusHistory = append(m.statusHistory, status)
	return json.Marshal(map[string]any{"id": id, "status": status, "reviewed_by": reviewer, "review_note": note})
}

func TestL4CascadeUsecase_Approve_CriticalFailCompensatesPriorSteps(t *testing.T) {
	repo := &mockL4GraphRepo{}
	store := &cascadeCompensateStore{
		cascadeApproveStore: cascadeApproveStore{
			proposal: []byte(`{
				"id":"cp1","agent_id":"ag1","status":"pending",
				"trigger_entity_id":"ent1","old_value":"Alice","new_value":"Bob",
				"affected_json":"[]"
			}`),
		},
		replaceErr: apierror.Internal("MEMORY", "forced replace failure"),
	}
	uc := NewL4CascadeUsecase(L4CascadeDeps{
		Proposals: store, Reader: store, Mutator: store, Saga: store,
		EntityWriter: repo, LG: loggateway.NewNoop(),
	})
	_, err := uc.Approve(context.Background(), "cp1", "reviewer-1")
	if err != nil {
		t.Fatal(err)
	}
	// UpsertEntity (step-1) succeeded then must be compensated after ReplaceFacts fails.
	if got := store.stepStates["step-1"]; got != "compensated" {
		t.Fatalf("expected step-1 compensated, got %q (states=%v)", got, store.stepStates)
	}
	if got := store.stepStates["step-3"]; got != "failed" {
		t.Fatalf("expected step-3 failed, got %q", got)
	}
	// Compensation restores old name via UpsertEntity.
	var restored bool
	for _, e := range repo.entities {
		if e.ID == "ent1" && e.Name == "Alice" {
			restored = true
		}
	}
	if !restored {
		t.Fatalf("expected entity restored to Alice after compensate; entities=%+v", repo.entities)
	}
	var sawPartial bool
	for _, s := range store.statusHistory {
		if s == "partial" {
			sawPartial = true
		}
	}
	if !sawPartial {
		t.Fatalf("expected proposal status partial after critical failure; history=%v", store.statusHistory)
	}
}

func TestL4CascadeUsecase_Approve_InitSagaFailRollsBackRunning(t *testing.T) {
	store := &cascadeInitFailStore{
		cascadeApproveStore: cascadeApproveStore{
			proposal: []byte(`{
				"id":"cp1","agent_id":"ag1","status":"pending",
				"trigger_entity_id":"ent1","old_value":"Alice","new_value":"Bob",
				"affected_json":"[]"
			}`),
		},
		initErr: apierror.Internal("MEMORY", "init saga failed"),
	}
	uc := NewL4CascadeUsecase(L4CascadeDeps{
		Proposals: store, Reader: store, Mutator: store, Saga: store,
		EntityWriter: &mockL4GraphRepo{}, LG: loggateway.NewNoop(),
	})
	_, err := uc.Approve(context.Background(), "cp1", "reviewer-1")
	if err == nil {
		t.Fatal("expected init error")
	}
	if len(store.statusHistory) == 0 || store.statusHistory[len(store.statusHistory)-1] != "pending" {
		t.Fatalf("expected rollback to pending, got history=%v", store.statusHistory)
	}
}

type cascadeInitFailStore struct {
	cascadeApproveStore
	initErr       error
	statusHistory []string
}

func (m *cascadeInitFailStore) InitCascadeSagaSteps(context.Context, string, []CascadeSagaStep) error {
	return m.initErr
}

func (m *cascadeInitFailStore) UpdateCascadeProposalStatus(_ context.Context, id, status, reviewer, note string) ([]byte, error) {
	m.statusHistory = append(m.statusHistory, status)
	return json.Marshal(map[string]any{"id": id, "status": status, "reviewed_by": reviewer, "review_note": note})
}

func TestL4CascadeUsecase_Reject_CAS(t *testing.T) {
	t.Run("pending_ok", func(t *testing.T) {
		store := &cascadeRejectStore{currentStatus: "pending", casOK: true}
		uc := NewL4CascadeUsecase(L4CascadeDeps{Proposals: store, Reader: store, Mutator: store, Saga: store, LG: loggateway.NewNoop()})
		raw, err := uc.Reject(context.Background(), "cp1", "r1", "nope")
		if err != nil {
			t.Fatal(err)
		}
		var row map[string]any
		if err := json.Unmarshal(raw, &row); err != nil {
			t.Fatal(err)
		}
		if row["status"] != "rejected" {
			t.Fatalf("got status %v", row["status"])
		}
	})
	t.Run("applied_blocked", func(t *testing.T) {
		store := &cascadeRejectStore{currentStatus: "applied", casOK: false}
		uc := NewL4CascadeUsecase(L4CascadeDeps{Proposals: store, Reader: store, Mutator: store, Saga: store, LG: loggateway.NewNoop()})
		_, err := uc.Reject(context.Background(), "cp1", "r1", "nope")
		if err != ErrCascadeRejectNotAllowed {
			t.Fatalf("expected ErrCascadeRejectNotAllowed, got %v", err)
		}
	})
}

type cascadeRejectStore struct {
	cascadeGraphStoreMock
	currentStatus string
	casOK         bool
}

func (m *cascadeRejectStore) CompareAndSwapProposalStatus(_ context.Context, id string, _ []string, toStatus, reviewer, note string) ([]byte, bool, error) {
	if !m.casOK {
		b, _ := json.Marshal(map[string]any{"id": id, "status": m.currentStatus})
		return b, false, nil
	}
	b, _ := json.Marshal(map[string]any{"id": id, "status": toStatus, "reviewed_by": reviewer, "review_note": note})
	return b, true, nil
}
