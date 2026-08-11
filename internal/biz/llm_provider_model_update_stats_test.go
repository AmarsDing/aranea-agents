package biz

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"aranea-agents/internal/biz/usage"
	"aranea-agents/pkg/loggateway"
)

// stubProviderModelWriterForUpdate echoes the merged model back as persisted.
type stubProviderModelWriterForUpdate struct {
	saved ProviderModel
}

func (s *stubProviderModelWriterForUpdate) CreateProviderModel(context.Context, ProviderModel) (ProviderModel, error) {
	return ProviderModel{}, nil
}
func (s *stubProviderModelWriterForUpdate) UpdateProviderModel(_ context.Context, m ProviderModel) (ProviderModel, error) {
	s.saved = m
	return m, nil
}
func (s *stubProviderModelWriterForUpdate) DeleteProviderModel(context.Context, string) error {
	return nil
}
func (s *stubProviderModelWriterForUpdate) UpdateProviderModelStatus(context.Context, string, string) error {
	return nil
}

// TestUpdate_InjectsStats verifies that Update decorates the returned row with
// the same 30-day usage stats as List/ListPaged. Regression: the admin UI
// replaces the whole row with the PATCH response after toggling enabled, so an
// undecorated response wiped usage_call_count_30d/model_hotness_score from the
// visible config_json until the next full reload.
func TestUpdate_InjectsStats(t *testing.T) {
	reader := &stubProviderModelReaderForDelete{
		model: ProviderModel{
			ID:         "m1",
			Key:        "openai:gpt-4o",
			Name:       "GPT-4o",
			Status:     "active",
			Enabled:    true,
			Provider:   "openai",
			Model:      "gpt-4o",
			ConfigJSON: `{"api_key_set":true}`,
		},
	}
	writer := &stubProviderModelWriterForUpdate{}
	statsReader := &fakeStatsReader{
		rows: []usage.BreakdownRow{
			{ProviderCode: "openai", ModelAPIID: "gpt-4o", CallCount: 7, TotalTokens: 700, TotalCostMicroUSD: 55, SuccessRate: 0.9, AvgLatencyMS: 120},
		},
	}
	inj := NewModelStatsInjector(statsReader, loggateway.NewNoop())
	inj.cacheTTL = time.Hour

	u := &LlmProviderModelUsecase{
		reader:        reader,
		writer:        writer,
		statsInjector: inj,
		lg:            loggateway.NewNoop(),
	}
	out, err := u.Update(context.Background(), "m1", ProviderModel{Enabled: false})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if writer.saved.Enabled {
		t.Fatal("merged model should have Enabled=false")
	}
	var cfg map[string]any
	if err := json.Unmarshal([]byte(out.ConfigJSON), &cfg); err != nil {
		t.Fatalf("unmarshal config: %v", err)
	}
	if v, _ := cfg["usage_call_count_30d"].(float64); v != 7 {
		t.Fatalf("usage_call_count_30d: expected 7, got %v", cfg["usage_call_count_30d"])
	}
	if _, ok := cfg["model_hotness_score"]; !ok {
		t.Fatal("model_hotness_score not injected")
	}
	if v, _ := cfg["api_key_set"].(bool); !v {
		t.Fatalf("api_key_set should be preserved, got %v", cfg["api_key_set"])
	}
}

// TestUpdate_NilStatsInjectorStillWorks keeps backward compatibility for
// consumers/tests that construct the usecase without a stats injector.
func TestUpdate_NilStatsInjectorStillWorks(t *testing.T) {
	reader := &stubProviderModelReaderForDelete{
		model: ProviderModel{ID: "m1", Key: "k", Name: "n", Status: "active", Provider: "openai", Model: "gpt-4o", ConfigJSON: `{"existing":true}`},
	}
	u := &LlmProviderModelUsecase{
		reader: reader,
		writer: &stubProviderModelWriterForUpdate{},
		lg:     loggateway.NewNoop(),
	}
	out, err := u.Update(context.Background(), "m1", ProviderModel{Enabled: false})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	var cfg map[string]any
	if err := json.Unmarshal([]byte(out.ConfigJSON), &cfg); err != nil {
		t.Fatalf("unmarshal config: %v", err)
	}
	if _, ok := cfg["usage_call_count_30d"]; ok {
		t.Fatal("usage_call_count_30d should not be set when statsInjector is nil")
	}
}
