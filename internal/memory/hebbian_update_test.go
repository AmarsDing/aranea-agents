package memory

import (
	"context"
	"errors"
	"testing"
	"time"

	"aranea-agents/internal/biz"
)

// mockHebbianStore is a test stub for biz.L4HebbianStore.
type mockHebbianStore struct {
	relation     biz.L4HebbianRelation
	found        bool
	findErr      error
	updateErr    error
	decayResult  biz.L4DecayResult
	decayErr     error
	lastUpdateID string
	lastWeight   float64
	lastCoAct    int
	lastReinf    string
	lastDecayArg string
}

func (m *mockHebbianStore) FindRelation(ctx context.Context, nodeA, nodeB, relationType string) (biz.L4HebbianRelation, bool, error) {
	if m.findErr != nil {
		return biz.L4HebbianRelation{}, false, m.findErr
	}
	return m.relation, m.found, nil
}

func (m *mockHebbianStore) UpdateRelationWeight(ctx context.Context, relationID string, newWeight float64, coActivationCount int, lastReinforcedAtRFC3339 string) error {
	m.lastUpdateID = relationID
	m.lastWeight = newWeight
	m.lastCoAct = coActivationCount
	m.lastReinf = lastReinforcedAtRFC3339
	return m.updateErr
}

func (m *mockHebbianStore) DecayUnusedRelations(ctx context.Context, olderThanRFC3339 string) (biz.L4DecayResult, error) {
	m.lastDecayArg = olderThanRFC3339
	return m.decayResult, m.decayErr
}

// TestHebbianUpdater_ReinforceConnection verifies the Hebbian rule:
// Δw = η * pre_activation * post_activation (η=0.1), weight saturated to [0, 1.0].
func TestHebbianUpdater_ReinforceConnection(t *testing.T) {
	// Existing relation: weight=0.5, source_activation=0.8, target_activation=0.6, co_act=2
	store := &mockHebbianStore{
		found: true,
		relation: biz.L4HebbianRelation{
			ID:                "rel-1",
			SourceID:          "A",
			TargetID:          "B",
			RelationType:      biz.RelationRelatedTo,
			Weight:            0.5,
			CoActivationCount: 2,
			SourceActivation:  0.8,
			TargetActivation:  0.6,
		},
	}
	updater := NewHebbianUpdater(store, nil)
	err := updater.ReinforceConnection(context.Background(), "A", "B", biz.RelationRelatedTo)
	if err != nil {
		t.Fatalf("ReinforceConnection: %v", err)
	}

	// Expected: newWeight = 0.5 + 0.1 * 0.8 * 0.6 = 0.5 + 0.048 = 0.548
	expectedWeight := 0.5 + 0.1*0.8*0.6
	if abs(store.lastWeight-expectedWeight) > 1e-9 {
		t.Errorf("weight: got %v, want %v", store.lastWeight, expectedWeight)
	}
	if store.lastCoAct != 3 {
		t.Errorf("co_activation_count: got %d, want 3", store.lastCoAct)
	}
	if store.lastUpdateID != "rel-1" {
		t.Errorf("relation ID: got %q, want rel-1", store.lastUpdateID)
	}
	if store.lastReinf == "" {
		t.Error("last_reinforced_at should be non-empty (now)")
	}
}

// TestHebbianUpdater_Saturation verifies weight is saturated to 1.0.
func TestHebbianUpdater_Saturation(t *testing.T) {
	// weight=0.95, source_activation=1.0, target_activation=1.0
	// newWeight = 0.95 + 0.1 * 1.0 * 1.0 = 1.05 → saturated to 1.0
	store := &mockHebbianStore{
		found: true,
		relation: biz.L4HebbianRelation{
			ID:                "rel-2",
			Weight:            0.95,
			CoActivationCount: 0,
			SourceActivation:  1.0,
			TargetActivation:  1.0,
		},
	}
	updater := NewHebbianUpdater(store, nil)
	err := updater.ReinforceConnection(context.Background(), "A", "B", biz.RelationRelatedTo)
	if err != nil {
		t.Fatalf("ReinforceConnection: %v", err)
	}
	if store.lastWeight > 1.0 {
		t.Errorf("weight should be saturated to 1.0, got %v", store.lastWeight)
	}
	if abs(store.lastWeight-1.0) > 1e-9 {
		t.Errorf("weight: got %v, want 1.0", store.lastWeight)
	}
}

// TestHebbianUpdater_RelationNotFound verifies graceful no-op when no relation exists.
func TestHebbianUpdater_RelationNotFound(t *testing.T) {
	store := &mockHebbianStore{found: false}
	updater := NewHebbianUpdater(store, nil)
	err := updater.ReinforceConnection(context.Background(), "A", "B", biz.RelationRelatedTo)
	if err != nil {
		t.Errorf("expected nil error for not-found relation, got %v", err)
	}
	if store.lastUpdateID != "" {
		t.Error("UpdateRelationWeight should not be called when relation not found")
	}
}

// TestHebbianUpdater_FindError verifies error propagation from FindRelation.
func TestHebbianUpdater_FindError(t *testing.T) {
	expectedErr := errors.New("DB connection lost")
	store := &mockHebbianStore{findErr: expectedErr}
	updater := NewHebbianUpdater(store, nil)
	err := updater.ReinforceConnection(context.Background(), "A", "B", biz.RelationRelatedTo)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err != expectedErr {
		t.Errorf("expected %v, got %v", expectedErr, err)
	}
}

// TestHebbianUpdater_UpdateError verifies error propagation from UpdateRelationWeight.
func TestHebbianUpdater_UpdateError(t *testing.T) {
	expectedErr := errors.New("write failed")
	store := &mockHebbianStore{
		found: true,
		relation: biz.L4HebbianRelation{
			ID:                "rel-3",
			Weight:            0.5,
			SourceActivation:  0.8,
			TargetActivation:  0.6,
		},
		updateErr: expectedErr,
	}
	updater := NewHebbianUpdater(store, nil)
	err := updater.ReinforceConnection(context.Background(), "A", "B", biz.RelationRelatedTo)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err != expectedErr {
		t.Errorf("expected %v, got %v", expectedErr, err)
	}
}

// TestHebbianUpdater_DecayUnused verifies that DecayUnused delegates to the store
// with a correct cutoff timestamp.
func TestHebbianUpdater_DecayUnused(t *testing.T) {
	store := &mockHebbianStore{
		decayResult: biz.L4DecayResult{Decayed: 10, Archived: 3},
	}
	updater := NewHebbianUpdater(store, nil)
	result, err := updater.DecayUnused(context.Background(), 24*time.Hour)
	if err != nil {
		t.Fatalf("DecayUnused: %v", err)
	}
	if result.Decayed != 10 || result.Archived != 3 {
		t.Errorf("decay result: got %+v, want {Decayed:10, Archived:3}", result)
	}
	if store.lastDecayArg == "" {
		t.Error("DecayUnusedRelations should be called with a non-empty cutoff")
	}
	// Verify the cutoff is approximately now - 24h (within 5 seconds tolerance).
	cutoff, parseErr := time.Parse(time.RFC3339, store.lastDecayArg)
	if parseErr != nil {
		t.Fatalf("cutoff is not valid RFC3339: %v", parseErr)
	}
	expectedCutoff := time.Now().UTC().Add(-24 * time.Hour)
	if abs(float64(cutoff.Sub(expectedCutoff).Seconds())) > 5 {
		t.Errorf("cutoff drift: got %v, want ~%v", cutoff, expectedCutoff)
	}
}

// TestHebbianUpdater_DecayUnusedError verifies error propagation.
func TestHebbianUpdater_DecayUnusedError(t *testing.T) {
	expectedErr := errors.New("decay query failed")
	store := &mockHebbianStore{decayErr: expectedErr}
	updater := NewHebbianUpdater(store, nil)
	_, err := updater.DecayUnused(context.Background(), 24*time.Hour)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err != expectedErr {
		t.Errorf("expected %v, got %v", expectedErr, err)
	}
}

// TestHebbianUpdater_NilUpdater verifies nil safety.
func TestHebbianUpdater_NilUpdater(t *testing.T) {
	var updater *HebbianUpdater
	err := updater.ReinforceConnection(context.Background(), "A", "B", biz.RelationRelatedTo)
	if err != nil {
		t.Errorf("expected nil error for nil updater, got %v", err)
	}
	_, err = updater.DecayUnused(context.Background(), 24*time.Hour)
	if err != nil {
		t.Errorf("expected nil error for nil updater, got %v", err)
	}
}

// TestHebbianUpdater_NilStore verifies nil store safety.
func TestHebbianUpdater_NilStore(t *testing.T) {
	updater := NewHebbianUpdater(nil, nil)
	err := updater.ReinforceConnection(context.Background(), "A", "B", biz.RelationRelatedTo)
	if err != nil {
		t.Errorf("expected nil error for nil store, got %v", err)
	}
}

// TestHebbianUpdater_LearningRate verifies the learning rate constant.
func TestHebbianUpdater_LearningRate(t *testing.T) {
	if hebbianLearningRate != 0.1 {
		t.Errorf("hebbianLearningRate: got %v, want 0.1", hebbianLearningRate)
	}
}

// TestHebbianUpdater_DecayFactor verifies the decay factor constant.
func TestHebbianUpdater_DecayFactor(t *testing.T) {
	if hebbianDecayFactor != 0.95 {
		t.Errorf("hebbianDecayFactor: got %v, want 0.95", hebbianDecayFactor)
	}
}

// TestHebbianUpdater_ArchiveThreshold verifies the archive threshold constant.
func TestHebbianUpdater_ArchiveThreshold(t *testing.T) {
	if hebbianArchiveThreshold != 0.1 {
		t.Errorf("hebbianArchiveThreshold: got %v, want 0.1", hebbianArchiveThreshold)
	}
}
