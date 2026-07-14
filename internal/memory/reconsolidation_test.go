package memory

import (
	"context"
	"errors"
	"testing"
	"time"

	"aranea-agents/internal/biz"
)

// mockReconsolidationStore is a test stub for biz.L4ReconsolidationStore.
type mockReconsolidationStore struct {
	boostOK       bool
	boostErr      error
	boostDelta    float64
	boostNodeID   string
	boostTimestamp string

	incrementOK    bool
	incrementErr   error
	incrementNodeID string
}

func (m *mockReconsolidationStore) BoostActivation(ctx context.Context, nodeID string, delta float64, nowRFC3339 string) (bool, error) {
	m.boostNodeID = nodeID
	m.boostDelta = delta
	m.boostTimestamp = nowRFC3339
	return m.boostOK, m.boostErr
}

func (m *mockReconsolidationStore) IncrementUseCount(ctx context.Context, nodeID string) (bool, error) {
	m.incrementNodeID = nodeID
	return m.incrementOK, m.incrementErr
}

// TestReconsolidation_OnRecall verifies the full reconsolidation flow:
// 1. Boost activation by 0.2 (saturated to 1.0)
// 2. Increment use_count
// 3. Hebbian reinforcement for each co-recalled neuron
func TestReconsolidation_OnRecall(t *testing.T) {
	store := &mockReconsolidationStore{
		boostOK:    true,
		incrementOK: true,
	}
	hebbianStore := &mockHebbianStore{
		found: true,
		relation: biz.L4HebbianRelation{
			ID:                "rel-1",
			Weight:            0.5,
			CoActivationCount: 1,
			SourceActivation:  0.8,
			TargetActivation:  0.6,
		},
	}
	hebbian := NewHebbianUpdater(hebbianStore, nil)
	svc := NewReconsolidationService(store, hebbian, nil)

	err := svc.OnRecall(context.Background(), "A", []string{"B", "C"})
	if err != nil {
		t.Fatalf("OnRecall: %v", err)
	}

	// Verify activation boost.
	if store.boostNodeID != "A" {
		t.Errorf("boost nodeID: got %q, want A", store.boostNodeID)
	}
	if abs(store.boostDelta-0.2) > 1e-9 {
		t.Errorf("boost delta: got %v, want 0.2", store.boostDelta)
	}
	if store.boostTimestamp == "" {
		t.Error("boost timestamp should be non-empty")
	}

	// Verify use_count increment.
	if store.incrementNodeID != "A" {
		t.Errorf("increment nodeID: got %q, want A", store.incrementNodeID)
	}

	// Verify Hebbian reinforcement was called for B and C.
	// The mock only records the last call, so we verify at least one call happened.
	if hebbianStore.lastUpdateID == "" {
		t.Error("Hebbian reinforcement should have been called")
	}
}

// TestReconsolidation_BoostDelta verifies the activation boost delta constant.
func TestReconsolidation_BoostDelta(t *testing.T) {
	if reconsolidationBoostDelta != 0.2 {
		t.Errorf("reconsolidationBoostDelta: got %v, want 0.2", reconsolidationBoostDelta)
	}
}

// TestReconsolidation_EntityNotFound verifies graceful handling when entity
// is not found (BoostActivation returns ok=false).
func TestReconsolidation_EntityNotFound(t *testing.T) {
	store := &mockReconsolidationStore{
		boostOK:    false, // entity not found
		incrementOK: true,
	}
	hebbian := NewHebbianUpdater(&mockHebbianStore{}, nil)
	svc := NewReconsolidationService(store, hebbian, nil)

	// Should not error — entity not found is a graceful no-op.
	err := svc.OnRecall(context.Background(), "X", nil)
	if err != nil {
		t.Errorf("expected nil error for not-found entity, got %v", err)
	}
}

// TestReconsolidation_BoostError verifies error propagation from BoostActivation.
func TestReconsolidation_BoostError(t *testing.T) {
	expectedErr := errors.New("DB write failed")
	store := &mockReconsolidationStore{
		boostOK:    true,
		boostErr:   expectedErr,
		incrementOK: true,
	}
	svc := NewReconsolidationService(store, NewHebbianUpdater(&mockHebbianStore{}, nil), nil)

	err := svc.OnRecall(context.Background(), "A", nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestReconsolidation_IncrementError verifies that IncrementUseCount error
// is logged but does not abort the flow (best-effort).
func TestReconsolidation_IncrementError(t *testing.T) {
	store := &mockReconsolidationStore{
		boostOK:    true,
		incrementOK: true,
		incrementErr: errors.New("increment failed"),
	}
	svc := NewReconsolidationService(store, NewHebbianUpdater(&mockHebbianStore{}, nil), nil)

	// Should not return error — increment failure is best-effort.
	err := svc.OnRecall(context.Background(), "A", nil)
	if err != nil {
		t.Errorf("expected nil error for increment failure (best-effort), got %v", err)
	}
}

// TestReconsolidation_HebbianError verifies that Hebbian reinforcement failure
// for one pair does not abort the flow (best-effort).
func TestReconsolidation_HebbianError(t *testing.T) {
	store := &mockReconsolidationStore{
		boostOK:    true,
		incrementOK: true,
	}
	hebbianStore := &mockHebbianStore{
		findErr: errors.New("find failed"),
	}
	svc := NewReconsolidationService(store, NewHebbianUpdater(hebbianStore, nil), nil)

	// Should not return error — Hebbian failure is best-effort.
	err := svc.OnRecall(context.Background(), "A", []string{"B", "C"})
	if err != nil {
		t.Errorf("expected nil error for Hebbian failure (best-effort), got %v", err)
	}
}

// TestReconsolidation_NilService verifies nil safety.
func TestReconsolidation_NilService(t *testing.T) {
	var svc *ReconsolidationService
	err := svc.OnRecall(context.Background(), "A", []string{"B"})
	if err != nil {
		t.Errorf("expected nil error for nil service, got %v", err)
	}
}

// TestReconsolidation_NilStore verifies nil store safety.
func TestReconsolidation_NilStore(t *testing.T) {
	svc := NewReconsolidationService(nil, nil, nil)
	err := svc.OnRecall(context.Background(), "A", []string{"B"})
	if err != nil {
		t.Errorf("expected nil error for nil store, got %v", err)
	}
}

// TestReconsolidation_EmptyRecalledWith verifies no Hebbian calls when
// recalledWith is empty.
func TestReconsolidation_EmptyRecalledWith(t *testing.T) {
	store := &mockReconsolidationStore{
		boostOK:    true,
		incrementOK: true,
	}
	hebbianStore := &mockHebbianStore{}
	svc := NewReconsolidationService(store, NewHebbianUpdater(hebbianStore, nil), nil)

	err := svc.OnRecall(context.Background(), "A", nil)
	if err != nil {
		t.Fatalf("OnRecall: %v", err)
	}
	if hebbianStore.lastUpdateID != "" {
		t.Error("Hebbian should not be called when recalledWith is empty")
	}
}

// TestReconsolidation_TimestampFormat verifies the timestamp is RFC3339.
func TestReconsolidation_TimestampFormat(t *testing.T) {
	store := &mockReconsolidationStore{
		boostOK:    true,
		incrementOK: true,
	}
	svc := NewReconsolidationService(store, NewHebbianUpdater(&mockHebbianStore{}, nil), nil)

	_ = svc.OnRecall(context.Background(), "A", nil)
	if store.boostTimestamp == "" {
		t.Fatal("boost timestamp should be non-empty")
	}
	if _, err := time.Parse(time.RFC3339, store.boostTimestamp); err != nil {
		t.Errorf("boost timestamp is not RFC3339: %v (err: %v)", store.boostTimestamp, err)
	}
}
