package biz

import (
	"context"
	"errors"
	"testing"

	"aranea-agents/pkg/loggateway"
)

// mockEcosystemPresetRepo implements EcosystemPresetRepo for testing.
type mockEcosystemPresetRepo struct {
	getEcosystemLoaded func(ctx context.Context) (EcosystemLoadedStatus, error)
	setEcosystemLoaded func(ctx context.Context, status EcosystemLoadedStatus) error
	deleteTaxonomy     func(ctx context.Context, industryKey string) (int, error)
	deleteAgents       func(ctx context.Context, industryKey string) (int, error)
	deleteTeams        func(ctx context.Context, industryKey string) (int, int, error)
}

func (m *mockEcosystemPresetRepo) GetEcosystemLoaded(ctx context.Context) (EcosystemLoadedStatus, error) {
	if m.getEcosystemLoaded != nil {
		return m.getEcosystemLoaded(ctx)
	}
	return nil, nil
}

func (m *mockEcosystemPresetRepo) SetEcosystemLoaded(ctx context.Context, status EcosystemLoadedStatus) error {
	if m.setEcosystemLoaded != nil {
		return m.setEcosystemLoaded(ctx, status)
	}
	return nil
}

func (m *mockEcosystemPresetRepo) DeleteTaxonomyNodesByIndustry(ctx context.Context, industryKey string) (int, error) {
	if m.deleteTaxonomy != nil {
		return m.deleteTaxonomy(ctx, industryKey)
	}
	return 0, nil
}

func (m *mockEcosystemPresetRepo) DeleteAgentsByIndustry(ctx context.Context, industryKey string) (int, error) {
	if m.deleteAgents != nil {
		return m.deleteAgents(ctx, industryKey)
	}
	return 0, nil
}

func (m *mockEcosystemPresetRepo) DeleteTeamsByIndustry(ctx context.Context, industryKey string) (int, int, error) {
	if m.deleteTeams != nil {
		return m.deleteTeams(ctx, industryKey)
	}
	return 0, 0, nil
}

// mockSeedPackFn returns a SeedPackFunc that records calls.
func mockSeedPackFn(results map[string][3]interface{}) SeedPackFunc {
	return func(ctx context.Context, client any, scenarioDir string, industryKey string, kindOverride string, lg loggateway.Logger) (int, int, error) {
		if r, ok := results[industryKey]; ok {
			return r[0].(int), r[1].(int), r[2].(error)
		}
		return 5, 1, nil // default: 5 agents, 1 team
	}
}

func newTestLogger() loggateway.Logger {
	return loggateway.NewNoop()
}

func TestNewEcosystemPresetUsecase(t *testing.T) {
	uc := NewEcosystemPresetUsecase(&mockEcosystemPresetRepo{}, mockSeedPackFn(nil), "internal/scenario", newTestLogger())
	if uc == nil {
		t.Fatal("expected non-nil EcosystemPresetUsecase")
	}
}

func TestEcosystemPresetUsecase_LoadEcosystemPreset_DefaultIndustries(t *testing.T) {
	repo := &mockEcosystemPresetRepo{
		getEcosystemLoaded: func(ctx context.Context) (EcosystemLoadedStatus, error) {
			return make(EcosystemLoadedStatus), nil
		},
		setEcosystemLoaded: func(ctx context.Context, status EcosystemLoadedStatus) error {
			// Verify all 3 default industries are loaded
			for _, ind := range DefaultIndustries {
				info, ok := status[ind]
				if !ok || !info.Loaded {
					t.Errorf("expected industry %s to be loaded", ind)
				}
			}
			return nil
		},
	}

	uc := NewEcosystemPresetUsecase(repo, mockSeedPackFn(nil), "internal/scenario", newTestLogger())
	resp, err := uc.LoadEcosystemPreset(context.Background(), nil, false, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(resp.Results))
	}
	if len(resp.AlreadyLoaded) != 0 {
		t.Fatalf("expected 0 already loaded, got %d", len(resp.AlreadyLoaded))
	}
}

func TestEcosystemPresetUsecase_LoadEcosystemPreset_AlreadyLoaded(t *testing.T) {
	repo := &mockEcosystemPresetRepo{
		getEcosystemLoaded: func(ctx context.Context) (EcosystemLoadedStatus, error) {
			return EcosystemLoadedStatus{
				"finance": {Loaded: true, LoadedAt: "2024-01-01T00:00:00Z"},
			}, nil
		},
		setEcosystemLoaded: func(ctx context.Context, status EcosystemLoadedStatus) error {
			return nil
		},
	}

	uc := NewEcosystemPresetUsecase(repo, mockSeedPackFn(nil), "internal/scenario", newTestLogger())
	resp, err := uc.LoadEcosystemPreset(context.Background(), []string{"finance"}, false, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.AlreadyLoaded) != 1 || resp.AlreadyLoaded[0] != "finance" {
		t.Fatalf("expected finance in already_loaded, got %v", resp.AlreadyLoaded)
	}
	if len(resp.Results) != 0 {
		t.Fatalf("expected 0 results, got %d", len(resp.Results))
	}
}

func TestEcosystemPresetUsecase_LoadEcosystemPreset_ForceReload(t *testing.T) {
	repo := &mockEcosystemPresetRepo{
		getEcosystemLoaded: func(ctx context.Context) (EcosystemLoadedStatus, error) {
			return EcosystemLoadedStatus{
				"finance": {Loaded: true, LoadedAt: "2024-01-01T00:00:00Z"},
			}, nil
		},
		setEcosystemLoaded: func(ctx context.Context, status EcosystemLoadedStatus) error {
			return nil
		},
	}

	uc := NewEcosystemPresetUsecase(repo, mockSeedPackFn(nil), "internal/scenario", newTestLogger())
	resp, err := uc.LoadEcosystemPreset(context.Background(), []string{"finance"}, true, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.AlreadyLoaded) != 0 {
		t.Fatalf("expected 0 already loaded with force, got %d", len(resp.AlreadyLoaded))
	}
	if len(resp.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(resp.Results))
	}
}

func TestEcosystemPresetUsecase_LoadEcosystemPreset_SeedError(t *testing.T) {
	seedFn := mockSeedPackFn(map[string][3]interface{}{
		"finance": {0, 0, errors.New("seed failed")},
	})
	repo := &mockEcosystemPresetRepo{
		getEcosystemLoaded: func(ctx context.Context) (EcosystemLoadedStatus, error) {
			return make(EcosystemLoadedStatus), nil
		},
		setEcosystemLoaded: func(ctx context.Context, status EcosystemLoadedStatus) error {
			return nil
		},
	}

	uc := NewEcosystemPresetUsecase(repo, seedFn, "internal/scenario", newTestLogger())
	resp, err := uc.LoadEcosystemPreset(context.Background(), []string{"finance"}, false, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Errors) != 1 {
		t.Fatalf("expected 1 error, got %d", len(resp.Errors))
	}
	if resp.Errors["finance"] != "seed failed" {
		t.Fatalf("expected 'seed failed' error, got %s", resp.Errors["finance"])
	}
}

func TestEcosystemPresetUsecase_LoadEcosystemPreset_GetStatusError(t *testing.T) {
	repo := &mockEcosystemPresetRepo{
		getEcosystemLoaded: func(ctx context.Context) (EcosystemLoadedStatus, error) {
			return nil, errors.New("db error")
		},
	}

	uc := NewEcosystemPresetUsecase(repo, mockSeedPackFn(nil), "internal/scenario", newTestLogger())
	_, err := uc.LoadEcosystemPreset(context.Background(), []string{"finance"}, false, nil)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestEcosystemPresetUsecase_UnloadEcosystemPreset_Success(t *testing.T) {
	repo := &mockEcosystemPresetRepo{
		getEcosystemLoaded: func(ctx context.Context) (EcosystemLoadedStatus, error) {
			return EcosystemLoadedStatus{
				"finance": {Loaded: true, LoadedAt: "2024-01-01T00:00:00Z"},
			}, nil
		},
		setEcosystemLoaded: func(ctx context.Context, status EcosystemLoadedStatus) error {
			info := status["finance"]
			if info.Loaded {
				t.Error("expected finance to be marked as not loaded")
			}
			return nil
		},
		deleteTaxonomy: func(ctx context.Context, industryKey string) (int, error) {
			return 5, nil
		},
		deleteAgents: func(ctx context.Context, industryKey string) (int, error) {
			return 3, nil
		},
		deleteTeams: func(ctx context.Context, industryKey string) (int, int, error) {
			return 1, 0, nil
		},
	}

	uc := NewEcosystemPresetUsecase(repo, mockSeedPackFn(nil), "internal/scenario", newTestLogger())
	resp, err := uc.UnloadEcosystemPreset(context.Background(), []string{"finance"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	result := resp.Results["finance"]
	if result == nil {
		t.Fatal("expected result for finance")
	}
	if result.AgentsDeleted != 3 {
		t.Fatalf("expected 3 agents deleted, got %d", result.AgentsDeleted)
	}
	if result.TeamsDeleted != 1 {
		t.Fatalf("expected 1 team deleted, got %d", result.TeamsDeleted)
	}
	if result.TaxonomyNodesDeleted != 5 {
		t.Fatalf("expected 5 taxonomy nodes deleted, got %d", result.TaxonomyNodesDeleted)
	}
}

func TestEcosystemPresetUsecase_UnloadEcosystemPreset_NotLoaded(t *testing.T) {
	repo := &mockEcosystemPresetRepo{
		getEcosystemLoaded: func(ctx context.Context) (EcosystemLoadedStatus, error) {
			return make(EcosystemLoadedStatus), nil
		},
	}

	uc := NewEcosystemPresetUsecase(repo, mockSeedPackFn(nil), "internal/scenario", newTestLogger())
	resp, err := uc.UnloadEcosystemPreset(context.Background(), []string{"finance"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Errors) != 1 || resp.Errors["finance"] != "industry not loaded" {
		t.Fatalf("expected 'industry not loaded' error, got %v", resp.Errors)
	}
}

func TestEcosystemPresetUsecase_UnloadEcosystemPreset_EmptyIndustries(t *testing.T) {
	uc := NewEcosystemPresetUsecase(&mockEcosystemPresetRepo{}, mockSeedPackFn(nil), "internal/scenario", newTestLogger())
	_, err := uc.UnloadEcosystemPreset(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error for empty industries")
	}
}

func TestEcosystemPresetUsecase_GetEcosystemStatus(t *testing.T) {
	expected := EcosystemLoadedStatus{
		"finance": {Loaded: true, LoadedAt: "2024-01-01T00:00:00Z"},
	}
	repo := &mockEcosystemPresetRepo{
		getEcosystemLoaded: func(ctx context.Context) (EcosystemLoadedStatus, error) {
			return expected, nil
		},
	}

	uc := NewEcosystemPresetUsecase(repo, mockSeedPackFn(nil), "internal/scenario", newTestLogger())
	status, err := uc.GetEcosystemStatus(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !status["finance"].Loaded {
		t.Fatal("expected finance to be loaded")
	}
}
