package modelcatalog

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
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
	cat := Catalog{
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
	res := NewApplier(backend).Apply(context.Background(), cat, "metadata_and_pricing")
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
	cat := Catalog{"openai": {ID: "openai", Models: map[string]Model{"gpt-4o": {ID: "gpt-4o"}}}}
	res := NewApplier(backend).Apply(context.Background(), cat, "metadata_and_pricing")
	if res.LLMRowsUpdated != 0 || len(backend.saved) != 0 {
		t.Fatalf("custom row should skip: saved=%d", len(backend.saved))
	}
}
