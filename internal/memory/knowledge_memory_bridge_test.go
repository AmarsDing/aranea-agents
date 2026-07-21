package memory

import (
	"context"
	"errors"
	"testing"

	"aranea-agents/internal/biz"
)

// mockKnowledgeBridgeStore is a test stub for biz.L4KnowledgeBridgeStore.
type mockKnowledgeBridgeStore struct {
	findEntities []biz.L4EntitySnapshot
	findErr      error
	findCallArg  string
	findCount    int

	adjustOK        bool
	adjustErr       error
	adjustIDs       []string // records all entityIDs adjusted
	adjustDeltas    []float64
	adjustFailOnNth int // 1-based; 0 = never fail; causes AdjustConfidence to error on Nth call
	adjustCallCount int
}

func (m *mockKnowledgeBridgeStore) FindBySourceCollection(ctx context.Context, collectionID string) ([]biz.L4EntitySnapshot, error) {
	m.findCount++
	m.findCallArg = collectionID
	return m.findEntities, m.findErr
}

func (m *mockKnowledgeBridgeStore) AdjustConfidence(ctx context.Context, entityID string, delta float64) (bool, error) {
	m.adjustCallCount++
	m.adjustIDs = append(m.adjustIDs, entityID)
	m.adjustDeltas = append(m.adjustDeltas, delta)
	if m.adjustFailOnNth > 0 && m.adjustCallCount == m.adjustFailOnNth {
		return m.adjustOK, m.adjustErr
	}
	return m.adjustOK, nil
}

// TestKnowledgeBridge_OnKnowledgeConfirmed verifies the full flow:
// FindBySourceCollection + AdjustConfidence(+0.1) for each entity.
func TestKnowledgeBridge_OnKnowledgeConfirmed(t *testing.T) {
	store := &mockKnowledgeBridgeStore{
		findEntities: []biz.L4EntitySnapshot{
			{ID: "E1", Confidence: 0.5},
			{ID: "E2", Confidence: 0.6},
		},
		adjustOK: true,
	}
	bridge := NewKnowledgeMemoryBridge(store, nil)

	err := bridge.OnKnowledgeConfirmed(context.Background(), "coll-1", "query", true)
	if err != nil {
		t.Fatalf("OnKnowledgeConfirmed: %v", err)
	}
	if store.findCallArg != "coll-1" {
		t.Errorf("FindBySourceCollection arg: got %q, want coll-1", store.findCallArg)
	}
	if len(store.adjustIDs) != 2 {
		t.Errorf("AdjustConfidence call count: got %d, want 2", len(store.adjustIDs))
	}
	for _, d := range store.adjustDeltas {
		if abs(d-0.1) > 1e-9 {
			t.Errorf("confirmed delta: got %v, want 0.1", d)
		}
	}
}

// TestKnowledgeBridge_OnKnowledgeRejected verifies rejected → -0.1 delta.
func TestKnowledgeBridge_OnKnowledgeRejected(t *testing.T) {
	store := &mockKnowledgeBridgeStore{
		findEntities: []biz.L4EntitySnapshot{{ID: "E1"}},
		adjustOK:     true,
	}
	bridge := NewKnowledgeMemoryBridge(store, nil)

	err := bridge.OnKnowledgeConfirmed(context.Background(), "coll-1", "query", false)
	if err != nil {
		t.Fatalf("OnKnowledgeConfirmed: %v", err)
	}
	if len(store.adjustDeltas) != 1 {
		t.Fatalf("adjust count: got %d, want 1", len(store.adjustDeltas))
	}
	if abs(store.adjustDeltas[0]-(-0.1)) > 1e-9 {
		t.Errorf("rejected delta: got %v, want -0.1", store.adjustDeltas[0])
	}
}

// TestKnowledgeBridge_KnowledgeConfirmBoostConstant verifies the +0.1 constant.
func TestKnowledgeBridge_KnowledgeConfirmBoostConstant(t *testing.T) {
	if knowledgeConfirmBoost != 0.1 {
		t.Errorf("knowledgeConfirmBoost: got %v, want 0.1", knowledgeConfirmBoost)
	}
}

// TestKnowledgeBridge_KnowledgeRejectPenaltyConstant verifies the -0.1 constant.
func TestKnowledgeBridge_KnowledgeRejectPenaltyConstant(t *testing.T) {
	if knowledgeRejectPenalty != -0.1 {
		t.Errorf("knowledgeRejectPenalty: got %v, want -0.1", knowledgeRejectPenalty)
	}
}

// TestKnowledgeBridge_FindError verifies error propagation from FindBySourceCollection.
func TestKnowledgeBridge_FindError(t *testing.T) {
	expectedErr := errors.New("query failed")
	store := &mockKnowledgeBridgeStore{
		findErr:  expectedErr,
		adjustOK: true,
	}
	bridge := NewKnowledgeMemoryBridge(store, nil)

	err := bridge.OnKnowledgeConfirmed(context.Background(), "coll-1", "query", true)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err != expectedErr {
		t.Errorf("expected %v, got %v", expectedErr, err)
	}
	if len(store.adjustIDs) != 0 {
		t.Errorf("AdjustConfidence should not be called when Find fails, got %d calls", len(store.adjustIDs))
	}
}

// TestKnowledgeBridge_AdjustErrorBestEffort verifies that AdjustConfidence
// failure is best-effort (logged but does not abort the loop).
func TestKnowledgeBridge_AdjustErrorBestEffort(t *testing.T) {
	store := &mockKnowledgeBridgeStore{
		findEntities: []biz.L4EntitySnapshot{
			{ID: "E1"},
			{ID: "E2"},
			{ID: "E3"},
		},
		adjustOK:        true,
		adjustErr:       errors.New("update failed"),
		adjustFailOnNth: 2, // 2nd call fails
	}
	bridge := NewKnowledgeMemoryBridge(store, nil)

	err := bridge.OnKnowledgeConfirmed(context.Background(), "coll-1", "query", true)
	if err != nil {
		t.Errorf("expected nil error (best-effort), got %v", err)
	}
	if len(store.adjustIDs) != 3 {
		t.Errorf("all 3 entities should be processed (best-effort), got %d", len(store.adjustIDs))
	}
}

// TestKnowledgeBridge_EmptyCollectionID verifies empty collectionID is a no-op.
func TestKnowledgeBridge_EmptyCollectionID(t *testing.T) {
	store := &mockKnowledgeBridgeStore{
		findEntities: []biz.L4EntitySnapshot{{ID: "E1"}},
		adjustOK:     true,
	}
	bridge := NewKnowledgeMemoryBridge(store, nil)

	err := bridge.OnKnowledgeConfirmed(context.Background(), "", "query", true)
	if err != nil {
		t.Errorf("expected nil for empty collectionID, got %v", err)
	}
	if store.findCount != 0 {
		t.Errorf("FindBySourceCollection should not be called for empty collectionID, got %d calls", store.findCount)
	}
}

// TestKnowledgeBridge_EmptyEntities verifies no AdjustConfidence calls when
// FindBySourceCollection returns empty list.
func TestKnowledgeBridge_EmptyEntities(t *testing.T) {
	store := &mockKnowledgeBridgeStore{
		findEntities: []biz.L4EntitySnapshot{},
		adjustOK:     true,
	}
	bridge := NewKnowledgeMemoryBridge(store, nil)

	err := bridge.OnKnowledgeConfirmed(context.Background(), "coll-1", "query", true)
	if err != nil {
		t.Fatalf("OnKnowledgeConfirmed: %v", err)
	}
	if len(store.adjustIDs) != 0 {
		t.Errorf("AdjustConfidence should not be called for empty entity list, got %d calls", len(store.adjustIDs))
	}
}

// TestKnowledgeBridge_NilBridge verifies nil safety.
func TestKnowledgeBridge_NilBridge(t *testing.T) {
	var bridge *KnowledgeMemoryBridge
	err := bridge.OnKnowledgeConfirmed(context.Background(), "coll-1", "query", true)
	if err != nil {
		t.Errorf("expected nil for nil bridge, got %v", err)
	}
	err = bridge.OnTaskFeedback(context.Background(), "agent-1", "success", []string{"E1"})
	if err != nil {
		t.Errorf("expected nil for nil bridge, got %v", err)
	}
}

// TestKnowledgeBridge_NilStore verifies nil store safety.
func TestKnowledgeBridge_NilStore(t *testing.T) {
	bridge := NewKnowledgeMemoryBridge(nil, nil)
	err := bridge.OnKnowledgeConfirmed(context.Background(), "coll-1", "query", true)
	if err != nil {
		t.Errorf("expected nil for nil store, got %v", err)
	}
}

// TestKnowledgeBridge_OnTaskFeedbackSuccess verifies task success → +0.1 for
// each related entity.
func TestKnowledgeBridge_OnTaskFeedbackSuccess(t *testing.T) {
	store := &mockKnowledgeBridgeStore{adjustOK: true}
	bridge := NewKnowledgeMemoryBridge(store, nil)

	err := bridge.OnTaskFeedback(context.Background(), "agent-1", "success", []string{"E1", "E2"})
	if err != nil {
		t.Fatalf("OnTaskFeedback: %v", err)
	}
	if len(store.adjustDeltas) != 2 {
		t.Fatalf("adjust count: got %d, want 2", len(store.adjustDeltas))
	}
	for _, d := range store.adjustDeltas {
		if abs(d-0.1) > 1e-9 {
			t.Errorf("success delta: got %v, want 0.1", d)
		}
	}
}

// TestKnowledgeBridge_OnTaskFeedbackFailure verifies task failure → -0.1.
func TestKnowledgeBridge_OnTaskFeedbackFailure(t *testing.T) {
	store := &mockKnowledgeBridgeStore{adjustOK: true}
	bridge := NewKnowledgeMemoryBridge(store, nil)

	err := bridge.OnTaskFeedback(context.Background(), "agent-1", "failure", []string{"E1"})
	if err != nil {
		t.Fatalf("OnTaskFeedback: %v", err)
	}
	if len(store.adjustDeltas) != 1 {
		t.Fatalf("adjust count: got %d, want 1", len(store.adjustDeltas))
	}
	if abs(store.adjustDeltas[0]-(-0.1)) > 1e-9 {
		t.Errorf("failure delta: got %v, want -0.1", store.adjustDeltas[0])
	}
}

// TestKnowledgeBridge_TaskSuccessBoostConstant verifies the +0.1 constant.
func TestKnowledgeBridge_TaskSuccessBoostConstant(t *testing.T) {
	if taskSuccessBoost != 0.1 {
		t.Errorf("taskSuccessBoost: got %v, want 0.1", taskSuccessBoost)
	}
}

// TestKnowledgeBridge_TaskFailurePenaltyConstant verifies the -0.1 constant.
func TestKnowledgeBridge_TaskFailurePenaltyConstant(t *testing.T) {
	if taskFailurePenalty != -0.1 {
		t.Errorf("taskFailurePenalty: got %v, want -0.1", taskFailurePenalty)
	}
}

// TestKnowledgeBridge_OnTaskFeedbackUnknownResult verifies unknown taskResult
// is a no-op.
func TestKnowledgeBridge_OnTaskFeedbackUnknownResult(t *testing.T) {
	store := &mockKnowledgeBridgeStore{adjustOK: true}
	bridge := NewKnowledgeMemoryBridge(store, nil)

	err := bridge.OnTaskFeedback(context.Background(), "agent-1", "unknown", []string{"E1"})
	if err != nil {
		t.Fatalf("OnTaskFeedback: %v", err)
	}
	if len(store.adjustIDs) != 0 {
		t.Errorf("AdjustConfidence should not be called for unknown result, got %d calls", len(store.adjustIDs))
	}
}

// TestKnowledgeBridge_OnTaskFeedbackEmptyEntities verifies no calls with empty
// relatedEntityIDs.
func TestKnowledgeBridge_OnTaskFeedbackEmptyEntities(t *testing.T) {
	store := &mockKnowledgeBridgeStore{adjustOK: true}
	bridge := NewKnowledgeMemoryBridge(store, nil)

	err := bridge.OnTaskFeedback(context.Background(), "agent-1", "success", nil)
	if err != nil {
		t.Fatalf("OnTaskFeedback: %v", err)
	}
	if len(store.adjustIDs) != 0 {
		t.Errorf("AdjustConfidence should not be called for empty entity list, got %d calls", len(store.adjustIDs))
	}
}

// TestKnowledgeBridge_OnTaskFeedbackBestEffort verifies AdjustConfidence
// failure is best-effort.
func TestKnowledgeBridge_OnTaskFeedbackBestEffort(t *testing.T) {
	store := &mockKnowledgeBridgeStore{
		adjustOK:        true,
		adjustErr:       errors.New("update failed"),
		adjustFailOnNth: 1,
	}
	bridge := NewKnowledgeMemoryBridge(store, nil)

	err := bridge.OnTaskFeedback(context.Background(), "agent-1", "success", []string{"E1", "E2"})
	if err != nil {
		t.Errorf("expected nil (best-effort), got %v", err)
	}
	if len(store.adjustIDs) != 2 {
		t.Errorf("all 2 entities should be processed (best-effort), got %d", len(store.adjustIDs))
	}
}
