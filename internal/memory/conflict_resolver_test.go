package memory

import (
	"context"
	"errors"
	"testing"

	"aranea-agents/internal/biz"
)

// mockConflictStore is a test stub for biz.L4ConflictStore.
type mockConflictStore struct {
	createOK        bool
	createErr       error
	lastCreate      biz.L4InhibitRelationCreate
	createCallCount int

	adjustOK        bool
	adjustErr       error
	lastAdjustID    string
	lastAdjustDelta float64
	adjustCallCount int
}

func (m *mockConflictStore) CreateInhibitRelation(ctx context.Context, params biz.L4InhibitRelationCreate) error {
	m.createCallCount++
	m.lastCreate = params
	return m.createErr
}

func (m *mockConflictStore) AdjustConfidence(ctx context.Context, entityID string, delta float64) (bool, error) {
	m.adjustCallCount++
	m.lastAdjustID = entityID
	m.lastAdjustDelta = delta
	return m.adjustOK, m.adjustErr
}

// TestConflictResolver_ResolveConflict verifies the full conflict resolution
// flow: CreateInhibitRelation (weight=0.8, type=INHIBIT) + AdjustConfidence
// (delta=-0.3).
func TestConflictResolver_ResolveConflict(t *testing.T) {
	store := &mockConflictStore{
		createOK: true,
		adjustOK: true,
	}
	resolver := NewConflictResolver(store, nil)

	err := resolver.ResolveConflict(context.Background(), "NEW", "OLD", "contradicts")
	if err != nil {
		t.Fatalf("ResolveConflict: %v", err)
	}

	// Verify CreateInhibitRelation was called with correct params.
	if store.createCallCount != 1 {
		t.Errorf("CreateInhibitRelation call count: got %d, want 1", store.createCallCount)
	}
	if store.lastCreate.SourceID != "NEW" {
		t.Errorf("SourceID: got %q, want NEW", store.lastCreate.SourceID)
	}
	if store.lastCreate.TargetID != "OLD" {
		t.Errorf("TargetID: got %q, want OLD", store.lastCreate.TargetID)
	}
	if abs(store.lastCreate.Weight-0.8) > 1e-9 {
		t.Errorf("Weight: got %v, want 0.8", store.lastCreate.Weight)
	}
	if store.lastCreate.ContextNote != "contradicts" {
		t.Errorf("ContextNote: got %q, want 'contradicts'", store.lastCreate.ContextNote)
	}

	// Verify AdjustConfidence was called on OLD entity with -0.3.
	if store.adjustCallCount != 1 {
		t.Errorf("AdjustConfidence call count: got %d, want 1", store.adjustCallCount)
	}
	if store.lastAdjustID != "OLD" {
		t.Errorf("AdjustConfidence entityID: got %q, want OLD", store.lastAdjustID)
	}
	if abs(store.lastAdjustDelta-(-0.3)) > 1e-9 {
		t.Errorf("AdjustConfidence delta: got %v, want -0.3", store.lastAdjustDelta)
	}
}

// TestConflictResolver_InhibitWeightConstant verifies the inhibit weight
// constant (strong inhibition).
func TestConflictResolver_InhibitWeightConstant(t *testing.T) {
	if inhibitWeight != 0.8 {
		t.Errorf("inhibitWeight: got %v, want 0.8", inhibitWeight)
	}
}

// TestConflictResolver_ConfidencePenaltyConstant verifies the confidence
// penalty constant applied to the old (suppressed) entity.
func TestConflictResolver_ConfidencePenaltyConstant(t *testing.T) {
	if confidencePenalty != -0.3 {
		t.Errorf("confidencePenalty: got %v, want -0.3", confidencePenalty)
	}
}

// TestConflictResolver_DefaultWeight verifies that when params.Weight is zero,
// the default inhibitWeight (0.8) is applied.
func TestConflictResolver_DefaultWeight(t *testing.T) {
	store := &mockConflictStore{createOK: true, adjustOK: true}
	resolver := NewConflictResolver(store, nil)

	// Manually call with zero weight via a custom params path —
	// ResolveConflict always uses inhibitWeight, so this test verifies that
	// the constant flows through.
	_ = resolver.ResolveConflict(context.Background(), "A", "B", "reason")
	if abs(store.lastCreate.Weight-inhibitWeight) > 1e-9 {
		t.Errorf("Weight: got %v, want %v (inhibitWeight)", store.lastCreate.Weight, inhibitWeight)
	}
}

// TestConflictResolver_CreateInhibitError verifies that a CreateInhibitRelation
// failure aborts the flow and AdjustConfidence is not called.
func TestConflictResolver_CreateInhibitError(t *testing.T) {
	expectedErr := errors.New("insert failed")
	store := &mockConflictStore{
		createOK:  false,
		createErr: expectedErr,
		adjustOK:  true,
	}
	resolver := NewConflictResolver(store, nil)

	err := resolver.ResolveConflict(context.Background(), "NEW", "OLD", "reason")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err != expectedErr {
		t.Errorf("expected %v, got %v", expectedErr, err)
	}
	if store.adjustCallCount != 0 {
		t.Errorf("AdjustConfidence should not be called when CreateInhibitRelation fails, got %d calls", store.adjustCallCount)
	}
}

// TestConflictResolver_AdjustConfidenceError verifies that an AdjustConfidence
// failure propagates the error.
func TestConflictResolver_AdjustConfidenceError(t *testing.T) {
	expectedErr := errors.New("update failed")
	store := &mockConflictStore{
		createOK:  true,
		adjustOK:  true,
		adjustErr: expectedErr,
	}
	resolver := NewConflictResolver(store, nil)

	err := resolver.ResolveConflict(context.Background(), "NEW", "OLD", "reason")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err != expectedErr {
		t.Errorf("expected %v, got %v", expectedErr, err)
	}
}

// TestConflictResolver_AdjustConfidenceNotFound verifies that when
// AdjustConfidence returns ok=false (entity not found), the call is treated as
// a graceful no-op (returns nil) — the entity may have been concurrently
// deleted.
func TestConflictResolver_AdjustConfidenceNotFound(t *testing.T) {
	store := &mockConflictStore{
		createOK: true,
		adjustOK: false, // entity not found
	}
	resolver := NewConflictResolver(store, nil)

	err := resolver.ResolveConflict(context.Background(), "NEW", "OLD", "reason")
	if err != nil {
		t.Errorf("expected nil error for not-found entity (best-effort), got %v", err)
	}
}

// TestConflictResolver_NilResolver verifies nil safety.
func TestConflictResolver_NilResolver(t *testing.T) {
	var resolver *ConflictResolver
	err := resolver.ResolveConflict(context.Background(), "A", "B", "reason")
	if err != nil {
		t.Errorf("expected nil error for nil resolver, got %v", err)
	}
}

// TestConflictResolver_NilStore verifies nil store safety.
func TestConflictResolver_NilStore(t *testing.T) {
	resolver := NewConflictResolver(nil, nil)
	err := resolver.ResolveConflict(context.Background(), "A", "B", "reason")
	if err != nil {
		t.Errorf("expected nil error for nil store, got %v", err)
	}
}

// TestConflictResolver_SameEntity verifies that self-inhibition
// (newEntityID == oldEntityID) is rejected.
func TestConflictResolver_SameEntity(t *testing.T) {
	store := &mockConflictStore{createOK: true, adjustOK: true}
	resolver := NewConflictResolver(store, nil)

	err := resolver.ResolveConflict(context.Background(), "X", "X", "self")
	if err == nil {
		t.Fatal("expected error for self-inhibition, got nil")
	}
	if store.createCallCount != 0 {
		t.Errorf("CreateInhibitRelation should not be called for self-inhibition, got %d calls", store.createCallCount)
	}
	if store.adjustCallCount != 0 {
		t.Errorf("AdjustConfidence should not be called for self-inhibition, got %d calls", store.adjustCallCount)
	}
}

// TestConflictResolver_EmptyIDs verifies that empty entity IDs are rejected.
func TestConflictResolver_EmptyIDs(t *testing.T) {
	store := &mockConflictStore{createOK: true, adjustOK: true}
	resolver := NewConflictResolver(store, nil)

	cases := []struct {
		name  string
		newID string
		oldID string
	}{
		{"empty new", "", "OLD"},
		{"empty old", "NEW", ""},
		{"both empty", "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := resolver.ResolveConflict(context.Background(), c.newID, c.oldID, "reason")
			if err == nil {
				t.Errorf("expected error for %s, got nil", c.name)
			}
		})
	}
	if store.createCallCount != 0 {
		t.Errorf("CreateInhibitRelation should not be called for empty IDs, got %d calls", store.createCallCount)
	}
}

// TestConflictResolver_VerifyInhibitRelationType verifies that the relation
// type passed to CreateInhibitRelation is INHIBIT.
func TestConflictResolver_VerifyInhibitRelationType(t *testing.T) {
	// Note: CreateInhibitRelation in the store interface is dedicated to
	// INHIBIT relations, so the store implementation enforces the type.
	// This test verifies the resolver passes through the dedicated method
	// (not a generic CreateRelation with a type parameter).
	store := &mockConflictStore{createOK: true, adjustOK: true}
	resolver := NewConflictResolver(store, nil)

	_ = resolver.ResolveConflict(context.Background(), "A", "B", "reason")
	// The mock records all params; we just verify the call happened.
	if store.createCallCount != 1 {
		t.Errorf("expected 1 CreateInhibitRelation call, got %d", store.createCallCount)
	}
}
