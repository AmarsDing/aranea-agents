package biz

import (
	"context"
	"encoding/json"
	"testing"
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

func (m *cascadeGraphStoreMock) UpdateSagaStepState(_ context.Context, _ int64, _, _ string) error {
	return nil
}

func (m *cascadeGraphStoreMock) UpdateSagaStepResult(_ context.Context, _ int64, _ string) error {
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
	uc := NewL4CascadeUsecase(store, store, store, store, nil)
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
	uc := NewL4CascadeUsecase(store, store, store, store, nil)
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

func (m *cascadeApproveStore) HasCascadeSaga(_ context.Context, _ string) (bool, error) {
	return false, nil
}

func (m *cascadeApproveStore) InitCascadeSagaSteps(_ context.Context, _ string, _ []CascadeSagaStep) error {
	return nil
}

func (m *cascadeApproveStore) GetCascadeSagaSteps(_ context.Context, proposalID string) ([]CascadeSagaStep, error) {
	return []CascadeSagaStep{
		{ID: 1, ProposalID: proposalID, StepIndex: 0, StepName: SagaStepUpsertEntity, State: "pending", IsCritical: true},
		{ID: 2, ProposalID: proposalID, StepIndex: 1, StepName: SagaStepTouchAffected, State: "pending", IsCritical: false},
		{ID: 3, ProposalID: proposalID, StepIndex: 2, StepName: SagaStepReplaceFacts, State: "pending", IsCritical: true},
		{ID: 4, ProposalID: proposalID, StepIndex: 3, StepName: SagaStepSyncIndex, State: "pending", IsCritical: false},
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

func (m *cascadeApproveStore) UpdateSagaStepState(_ context.Context, _ int64, _, _ string) error {
	return nil
}

func (m *cascadeApproveStore) UpdateSagaStepResult(_ context.Context, _ int64, _ string) error {
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
	uc := NewL4CascadeUsecase(store, store, store, store, repo)
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
