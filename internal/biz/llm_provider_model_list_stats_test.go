package biz

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"aranea-agents/internal/biz/usage"
	"aranea-agents/pkg/loggateway"
)

// stubProviderModelReaderForList returns canned items for List tests.
type stubProviderModelReaderForList struct {
	items []ProviderModel
	err   error
}

func (s *stubProviderModelReaderForList) ListProviderModels(context.Context) ([]ProviderModel, error) {
	return s.items, s.err
}
func (s *stubProviderModelReaderForList) SearchProviderModels(_ context.Context, q ProviderModelListQuery) (ProviderModelListResult, error) {
	if s.err != nil {
		return ProviderModelListResult{}, s.err
	}
	return ProviderModelListResult{Items: s.items, Total: len(s.items), Limit: q.Limit, Offset: q.Offset}, nil
}
func (s *stubProviderModelReaderForList) GetProviderModel(context.Context, string) (ProviderModel, error) {
	return ProviderModel{}, nil
}
func (s *stubProviderModelReaderForList) GetProviderModelByProviderAndModel(context.Context, string, string) (ProviderModel, error) {
	return ProviderModel{}, nil
}

// TestList_InjectsStats verifies that List invokes statsInjector.InjectStats to
// populate 30-day usage / hotness fields into ProviderModel.ConfigJSON.
// RED: fails because LlmProviderModelUsecase has no statsInjector field yet.
func TestList_InjectsStats(t *testing.T) {
	reader := &stubProviderModelReaderForList{
		items: []ProviderModel{
			{Provider: "openai", Model: "gpt-4o", ConfigJSON: `{"api_key_set":true}`},
		},
	}
	statsReader := &fakeStatsReader{
		rows: []usage.BreakdownRow{
			{ProviderCode: "openai", ModelAPIID: "gpt-4o", CallCount: 42, TotalTokens: 4200, TotalCostMicroUSD: 100, SuccessRate: 0.95, AvgLatencyMS: 250},
		},
	}
	inj := NewModelStatsInjector(statsReader, loggateway.NewNoop())
	inj.cacheTTL = 1 * time.Hour

	u := &LlmProviderModelUsecase{
		reader:        reader,
		statsInjector: inj,
		lg:            loggateway.NewNoop(),
	}
	items, err := u.List(context.Background())
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	var cfg map[string]any
	if err := json.Unmarshal([]byte(items[0].ConfigJSON), &cfg); err != nil {
		t.Fatalf("unmarshal config: %v", err)
	}
	if v, _ := cfg["usage_call_count_30d"].(float64); v != 42 {
		t.Fatalf("usage_call_count_30d: expected 42, got %v", cfg["usage_call_count_30d"])
	}
	if v, _ := cfg["usage_total_tokens_30d"].(float64); v != 4200 {
		t.Fatalf("usage_total_tokens_30d: expected 4200, got %v", cfg["usage_total_tokens_30d"])
	}
	if v, _ := cfg["success_rate_30d"].(float64); v != 0.95 {
		t.Fatalf("success_rate_30d: expected 0.95, got %v", cfg["success_rate_30d"])
	}
	if v, _ := cfg["avg_latency_ms_30d"].(float64); v != 250 {
		t.Fatalf("avg_latency_ms_30d: expected 250, got %v", cfg["avg_latency_ms_30d"])
	}
	if _, ok := cfg["model_hotness_score"]; !ok {
		t.Fatalf("model_hotness_score not set")
	}
	// api_key_set 应保留
	if v, _ := cfg["api_key_set"].(bool); !v {
		t.Fatalf("api_key_set should be preserved, got %v", cfg["api_key_set"])
	}
}

// TestList_NilStatsInjectorStillWorks verifies List works when statsInjector is nil
// (backward compatibility: tests with empty Usecase must still pass).
func TestList_NilStatsInjectorStillWorks(t *testing.T) {
	reader := &stubProviderModelReaderForList{
		items: []ProviderModel{
			{Provider: "openai", Model: "gpt-4o", ConfigJSON: `{"existing":true}`},
		},
	}
	u := &LlmProviderModelUsecase{
		reader: reader,
		lg:     loggateway.NewNoop(),
	}
	items, err := u.List(context.Background())
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	var cfg map[string]any
	if err := json.Unmarshal([]byte(items[0].ConfigJSON), &cfg); err != nil {
		t.Fatalf("unmarshal config: %v", err)
	}
	if _, ok := cfg["usage_call_count_30d"]; ok {
		t.Fatal("usage_call_count_30d should not be set when statsInjector is nil")
	}
}
