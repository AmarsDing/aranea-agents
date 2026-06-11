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

func (m *mockL4GraphRepo) GetEntitiesByType(_ context.Context, _, _ string) ([]L4Entity, error) {
	return nil, nil
}

func (m *mockL4GraphRepo) GetEntityRelations(_ context.Context, _ string) ([]L4Relation, error) {
	return nil, nil
}

func (m *mockL4GraphRepo) SearchEntitiesByName(_ context.Context, _, _ string, _ int) ([]L4Entity, error) {
	return nil, nil
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
	uc.SetCascade(NewL4CascadeUsecase(L4CascadeDeps{Proposals: &cascadeGraphStoreMock{}, Reader: &cascadeGraphStoreMock{}, Mutator: &cascadeGraphStoreMock{}, Saga: &cascadeGraphStoreMock{}, LG: loggateway.NewNoop()}))
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

// --- Bug fix tests (3E-1) ---

func TestL4ChineseNamePattern_DoesNotMatchNonName(t *testing.T) {
	// "我是学生" should NOT match — "我是" is no longer in the pattern
	matches := l4ChineseNamePattern.FindStringSubmatch("我是学生")
	if len(matches) > 1 {
		t.Errorf("l4ChineseNamePattern should not match '我是学生', got %q", matches[1])
	}
	// "我是来帮忙的" should NOT match
	matches = l4ChineseNamePattern.FindStringSubmatch("我是来帮忙的")
	if len(matches) > 1 {
		t.Errorf("l4ChineseNamePattern should not match '我是来帮忙的', got %q", matches[1])
	}
}

func TestL4ChineseNamePattern_MatchesActualName(t *testing.T) {
	tests := []struct {
		input string
		name  string
	}{
		{"我叫小明", "小明"},
		{"我的名字是小红", "小红"},
	}
	for _, tt := range tests {
		matches := l4ChineseNamePattern.FindStringSubmatch(tt.input)
		if len(matches) < 2 || matches[1] != tt.name {
			t.Errorf("l4ChineseNamePattern(%q) = %v, want name=%q", tt.input, matches, tt.name)
		}
	}
}

func TestL4DecayConfig_AlphaRetention(t *testing.T) {
	cfg := DefaultL4DecayConfig()
	// Alpha=0.15 means retention factor = 1 - 0.15 = 0.85
	retention := 1.0 - cfg.Alpha
	if retention != 0.85 {
		t.Errorf("retention factor = %f, want 0.85", retention)
	}
	// A confidence of 0.8 after one decay period should be 0.8 * 0.85 = 0.68
	conf := 0.8 * retention
	if conf < 0.67 || conf > 0.69 {
		t.Errorf("0.8 * retention = %f, want ~0.68", conf)
	}
}

func TestL4ChinesePreferencePattern(t *testing.T) {
	tests := []struct {
		input string
		pref  string
	}{
		{"我喜欢暗色模式", "暗色模式"},
		{"我爱喝咖啡", "咖啡"},
	}
	for _, tt := range tests {
		matches := l4ChinesePreferencePattern.FindStringSubmatch(tt.input)
		if len(matches) < 2 || matches[1] != tt.pref {
			t.Errorf("l4ChinesePreferencePattern(%q) = %v, want pref=%q", tt.input, matches, tt.pref)
		}
	}
}
