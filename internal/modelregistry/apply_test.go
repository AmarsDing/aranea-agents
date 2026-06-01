package modelregistry

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"aranea-agents/pkg/loggateway"
)

type stubApplyBackend struct {
	rows    []ApplyRow
	saved   []ApplyRow
	pricing []string
	migrate map[string]ApplyMigrationStats
}

func (s *stubApplyBackend) ListProviderModels(ctx context.Context) ([]ApplyRow, error) {
	return s.rows, nil
}

func (s *stubApplyBackend) SaveProviderModel(ctx context.Context, row ApplyRow) error {
	s.saved = append(s.saved, row)
	return nil
}

func (s *stubApplyBackend) UpsertModelPricing(ctx context.Context, provider, model string, micro MicroPricing, source string) error {
	s.pricing = append(s.pricing, provider+":"+model)
	return nil
}

func (s *stubApplyBackend) CountProviderBindings(ctx context.Context, provider string) (ApplyMigrationStats, error) {
	return ApplyMigrationStats{}, nil
}

func (s *stubApplyBackend) MigrateProviderBindings(ctx context.Context, from, to string) (ApplyMigrationStats, error) {
	if s.migrate == nil {
		s.migrate = map[string]ApplyMigrationStats{}
	}
	key := from + "->" + to
	stats := s.migrate[key]
	s.migrate[key] = stats
	return stats, nil
}

func (s *stubApplyBackend) BatchMigrateProviderBindings(ctx context.Context, rules []ProviderMigrationRule, skipRules []string) BatchMigrationResult {
	return BatchMigrationResult{}
}

func (s *stubApplyBackend) BatchApply(ctx context.Context, patches []ApplyRow, pricing []PricingUpsert) BatchApplyResult {
	return BatchApplyResult{}
}

func TestApplier_metadataAndPricing(t *testing.T) {
	backend := &stubApplyBackend{
		rows: []ApplyRow{{
			ID:       "1",
			Key:      "openai:gpt-4o",
			Provider: "openai",
			Model:    "gpt-4o",
			Enabled:  true,
			ConfigJSON: `{"catalog_managed":true,"catalog_source":"models.dev"}`,
		}},
	}
	cat := Directory{
		"openai": {
			ID:   "openai",
			Name: "OpenAI",
			Env:  []string{"OPENAI_API_KEY"},
			Models: map[string]Model{
				"gpt-4o": {
					ID:          "gpt-4o",
					Name:        "GPT-4o",
					ReleaseDate: "2024-05-13",
					Cost:        &ModelCost{Input: 2.5, Output: 10, CacheRead: 1.25},
					Limit:       ModelLimit{Context: 128000, Output: 4096},
				},
			},
		},
	}
	res := NewApplier(backend, loggateway.NewNoop()).Apply(context.Background(), cat, "metadata_and_pricing")
	if res.LLMRowsUpdated != 1 {
		t.Fatalf("expected 1 update, got %d errors=%v", res.LLMRowsUpdated, res.Errors)
	}
	if len(backend.saved) != 1 {
		t.Fatal("expected save")
	}
	var meta map[string]any
	if err := json.Unmarshal([]byte(backend.saved[0].MetadataJSON), &meta); err != nil {
		t.Fatal(err)
	}
	if meta["release_date"] != "2024-05-13" {
		t.Fatalf("release_date missing: %#v", meta)
	}
	env, _ := meta["catalog_env"].([]any)
	if len(env) != 1 {
		t.Fatalf("catalog_env: %#v", meta["catalog_env"])
	}
	if len(backend.pricing) != 1 || !strings.Contains(backend.pricing[0], "openai:gpt-4o") {
		t.Fatalf("pricing: %v", backend.pricing)
	}
}

func TestApplier_skipsCustom(t *testing.T) {
	backend := &stubApplyBackend{
		rows: []ApplyRow{{
			ID:         "1",
			Provider:   "openai",
			Model:      "gpt-4o",
			ConfigJSON: `{"catalog_source":"custom"}`,
		}},
	}
	cat := Directory{"openai": {ID: "openai", Models: map[string]Model{"gpt-4o": {ID: "gpt-4o"}}}}
	res := NewApplier(backend, loggateway.NewNoop()).Apply(context.Background(), cat, "metadata_and_pricing")
	if res.LLMRowsUpdated != 0 || len(backend.saved) != 0 {
		t.Fatalf("custom row should skip: saved=%d", len(backend.saved))
	}
}

func TestApplier_ApplyWithMigration(t *testing.T) {
	backend := &stubApplyBackend{
		rows: []ApplyRow{{
			ID:         "1",
			Key:        "openai:gpt-4o",
			Provider:   "openai",
			Model:      "gpt-4o",
			Enabled:    true,
			ConfigJSON: `{"catalog_managed":true,"catalog_source":"models.dev"}`,
		}},
		migrate: map[string]ApplyMigrationStats{
			"gemini->google": {Agents: 3, Sessions: 1},
		},
	}
	cat := Directory{
		"openai": {
			ID:   "openai",
			Name: "OpenAI",
			Models: map[string]Model{
				"gpt-4o": {ID: "gpt-4o", Name: "GPT-4o"},
			},
		},
	}
	res := NewApplier(backend, loggateway.NewNoop()).ApplyWithMigration(context.Background(), cat, "metadata_and_pricing")
	if res.LLMRowsUpdated != 1 {
		t.Errorf("expected 1 LLM row updated, got %d", res.LLMRowsUpdated)
	}
	if res.Migration.Agents == 0 {
		t.Error("expected migration stats to be populated")
	}
}

func TestApplier_ApplyWithMigration_noneMode(t *testing.T) {
	backend := &stubApplyBackend{
		rows: []ApplyRow{{
			ID:         "1",
			Provider:   "openai",
			Model:      "gpt-4o",
			ConfigJSON: `{"catalog_managed":true}`,
		}},
	}
	cat := Directory{"openai": {ID: "openai", Models: map[string]Model{"gpt-4o": {ID: "gpt-4o"}}}}
	res := NewApplier(backend, loggateway.NewNoop()).ApplyWithMigration(context.Background(), cat, "none")
	if res.LLMRowsUpdated != 0 {
		t.Errorf("none mode should skip apply, got %d", res.LLMRowsUpdated)
	}
}

func TestApplier_Apply_emptyDirectory(t *testing.T) {
	backend := &stubApplyBackend{}
	res := NewApplier(backend, loggateway.NewNoop()).Apply(context.Background(), nil, "metadata_and_pricing")
	if res.LLMRowsUpdated != 0 {
		t.Errorf("empty directory should produce 0 updates, got %d", res.LLMRowsUpdated)
	}
}

func TestApplier_Apply_noneMode(t *testing.T) {
	backend := &stubApplyBackend{
		rows: []ApplyRow{{ID: "1", Provider: "openai", Model: "gpt-4o", ConfigJSON: `{}`}},
	}
	cat := Directory{"openai": {ID: "openai", Models: map[string]Model{"gpt-4o": {ID: "gpt-4o"}}}}
	res := NewApplier(backend, loggateway.NewNoop()).Apply(context.Background(), cat, "")
	if res.LLMRowsUpdated != 0 {
		t.Errorf("empty mode should skip, got %d", res.LLMRowsUpdated)
	}
}

func TestApplier_Apply_deprecatedModel(t *testing.T) {
	backend := &stubApplyBackend{
		rows: []ApplyRow{{
			ID:         "1",
			Key:        "openai:gpt-3.5",
			Provider:   "openai",
			Model:      "gpt-3.5",
			Enabled:    true,
			ConfigJSON: `{"catalog_managed":true,"catalog_source":"models.dev"}`,
		}},
	}
	cat := Directory{
		"openai": {
			ID:   "openai",
			Name: "OpenAI",
			Models: map[string]Model{
				"gpt-3.5": {ID: "gpt-3.5", Name: "GPT-3.5", Status: "deprecated"},
			},
		},
	}
	res := NewApplier(backend, loggateway.NewNoop()).Apply(context.Background(), cat, "metadata_and_pricing")
	if res.LLMRowsDisabled != 1 {
		t.Errorf("deprecated model should be disabled, got %d", res.LLMRowsDisabled)
	}
	if len(backend.saved) != 1 {
		t.Fatal("deprecated model should still be saved (with enabled=false)")
	}
	if backend.saved[0].Enabled {
		t.Error("deprecated model should have enabled=false")
	}
}
