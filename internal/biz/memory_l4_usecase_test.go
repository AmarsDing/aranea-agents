package biz

import (
	"context"
	"testing"

	"aranea-agents/pkg/loggateway"
)

type mockL4GraphRepo struct {
	entities   []L4EntityWrite
	relations  []L4RelationWrite
	entitySnap L4EntitySnapshot
	entityOk   bool
}

func (m *mockL4GraphRepo) UpsertEntity(_ context.Context, params L4EntityWrite) error {
	m.entities = append(m.entities, params)
	return nil
}

func (m *mockL4GraphRepo) UpsertRelation(_ context.Context, params L4RelationWrite) error {
	m.relations = append(m.relations, params)
	return nil
}

func (m *mockL4GraphRepo) GetEntityByScopeKey(_ context.Context, _, _, _, _ string) (L4EntitySnapshot, bool, error) {
	return m.entitySnap, m.entityOk, nil
}

func (m *mockL4GraphRepo) GetFirstEntityByType(_ context.Context, _, _, _ string) (L4EntitySnapshot, bool, error) {
	return m.entitySnap, m.entityOk, nil
}

func (m *mockL4GraphRepo) ApplyConfidenceDecay(_ context.Context, _, _, _ string, _ float64) (int64, error) {
	return 0, nil
}

func (m *mockL4GraphRepo) RecordEntityReinforcement(_ context.Context, _ string, _ ReinforcementSignal, _ string) error {
	return nil
}

func (m *mockL4GraphRepo) GetRecentReinforcementCounts(_ context.Context, _, _ string, _ int) (map[string]int, error) {
	return nil, nil
}

func (m *mockL4GraphRepo) ApplyBusinessConfidenceDecay(_ context.Context, _, _ string, _ L4DecayConfig, _ int64) (int64, error) {
	return 0, nil
}

func (m *mockL4GraphRepo) ArchiveLowConfidenceEntities(_ context.Context, _, _ string, _ float64) (int64, error) {
	return 0, nil
}

func TestL4GraphUsecase_WriteFromUserText_NilRepo(t *testing.T) {
	uc := NewL4GraphUsecase(nil, loggateway.NewNoop())
	n, err := uc.WriteFromUserText(context.Background(), "ag1", "u1", "My name is Alice")
	if err != nil || n != 0 {
		t.Fatalf("nil repo: n=%d err=%v", n, err)
	}
}

func TestL4GraphUsecase_WriteFromUserText_NameExtraction(t *testing.T) {
	repo := &mockL4GraphRepo{}
	uc := NewL4GraphUsecase(repo, loggateway.NewNoop())
	n, err := uc.WriteFromUserText(context.Background(), "ag1", "u1", "My name is Alice")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n < 1 {
		t.Fatalf("expected at least 1 write, got %d", n)
	}
	found := false
	for _, e := range repo.entities {
		if e.EntityType == "person" && e.Name == "Alice" {
			found = true
		}
	}
	if !found {
		t.Fatal("person entity not written")
	}
}

func TestL4GraphUsecase_WriteFromUserText_NameConflictGate(t *testing.T) {
	repo := &mockL4GraphRepo{
		entitySnap: L4EntitySnapshot{ID: "e1", Name: "Alice", Confidence: 0.8},
		entityOk:   true,
	}
	uc := NewL4GraphUsecase(repo, loggateway.NewNoop())
	uc.SetCascade(NewL4CascadeUsecase(&cascadeGraphStoreMock{}, &cascadeGraphStoreMock{}, &cascadeGraphStoreMock{}, &cascadeGraphStoreMock{}, nil, loggateway.NewNoop()))
	n, err := uc.WriteFromUserText(context.Background(), "ag1", "u1", "My name is Bob")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n < 1 {
		t.Fatalf("expected writes, got %d", n)
	}
	for _, e := range repo.entities {
		if e.EntityType == "person" && e.Name != "Alice" {
			t.Fatalf("conflict gate failed: person name=%q want Alice", e.Name)
		}
	}
}

func TestL4GraphUsecase_WriteFromUserText_PreferenceExtraction(t *testing.T) {
	repo := &mockL4GraphRepo{}
	uc := NewL4GraphUsecase(repo, loggateway.NewNoop())
	n, err := uc.WriteFromUserText(context.Background(), "ag1", "u1", "I prefer dark mode")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n < 1 {
		t.Fatalf("expected at least 1 write, got %d", n)
	}
	found := false
	for _, e := range repo.entities {
		if e.EntityType == "preference" {
			found = true
		}
	}
	if !found {
		t.Fatal("preference entity not written")
	}
}

func TestPreparePersonUpsert_conflict(t *testing.T) {
	uc := &L4GraphUsecase{}
	existing := L4EntitySnapshot{ID: "e1", Name: "Alice", Confidence: 0.8}
	prepared, conflict := uc.preparePersonUpsert(existing, "Bob", "My name is Bob")
	if !conflict {
		t.Fatal("expected conflict when name changes")
	}
	if prepared.MetadataJSON == "" {
		t.Fatal("expected metadata")
	}
}

func TestPreparePersonUpsert_noConflictSameName(t *testing.T) {
	uc := &L4GraphUsecase{}
	existing := L4EntitySnapshot{ID: "e1", Name: "Alice", Confidence: 0.8}
	_, conflict := uc.preparePersonUpsert(existing, "Alice", "Alice here")
	if conflict {
		t.Fatal("unexpected conflict for same name")
	}
}
