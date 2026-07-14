package biz

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"aranea-agents/internal/biz/usage"
	"aranea-agents/pkg/loggateway"
)

// fakeStatsReader 实现 ModelStatsReader，用于测试。
type fakeStatsReader struct {
	mu    sync.Mutex
	rows  []usage.BreakdownRow
	err   error
	calls int
}

func (f *fakeStatsReader) ListTopModelUsageFromDaily(ctx context.Context, q usage.Query) ([]usage.BreakdownRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	return f.rows, f.err
}

// TestInjectStats_InjectsFieldsIntoConfig 注入字段到 ConfigJSON。
func TestInjectStats_InjectsFieldsIntoConfig(t *testing.T) {
	reader := &fakeStatsReader{
		rows: []usage.BreakdownRow{
			{ProviderCode: "openai", ModelAPIID: "gpt-4o", CallCount: 100, TotalTokens: 1000, TotalCostMicroUSD: 5000, SuccessRate: 0.95, AvgLatencyMS: 200},
		},
	}
	inj := NewModelStatsInjector(reader, loggateway.NewNoop())
	// 缩短缓存 TTL 以便测试
	inj.cacheTTL = 1 * time.Hour

	items := []ProviderModel{
		{Provider: "openai", Model: "gpt-4o", ConfigJSON: `{"api_key_set":true}`},
	}
	inj.InjectStats(context.Background(), items)

	var cfg map[string]any
	if err := json.Unmarshal([]byte(items[0].ConfigJSON), &cfg); err != nil {
		t.Fatalf("unmarshal config: %v", err)
	}

	if v, _ := cfg["usage_call_count_30d"].(float64); v != 100 {
		t.Fatalf("usage_call_count_30d: expected 100, got %v", cfg["usage_call_count_30d"])
	}
	if v, _ := cfg["usage_total_tokens_30d"].(float64); v != 1000 {
		t.Fatalf("usage_total_tokens_30d: expected 1000, got %v", cfg["usage_total_tokens_30d"])
	}
	if v, _ := cfg["usage_cost_micro_usd_30d"].(float64); v != 5000 {
		t.Fatalf("usage_cost_micro_usd_30d: expected 5000, got %v", cfg["usage_cost_micro_usd_30d"])
	}
	if v, _ := cfg["success_rate_30d"].(float64); v != 0.95 {
		t.Fatalf("success_rate_30d: expected 0.95, got %v", cfg["success_rate_30d"])
	}
	if v, _ := cfg["avg_latency_ms_30d"].(float64); v != 200 {
		t.Fatalf("avg_latency_ms_30d: expected 200, got %v", cfg["avg_latency_ms_30d"])
	}
	if _, ok := cfg["model_hotness_score"]; !ok {
		t.Fatalf("model_hotness_score not set")
	}
	// api_key_set 应该保留
	if v, _ := cfg["api_key_set"].(bool); !v {
		t.Fatalf("api_key_set should be preserved, got %v", cfg["api_key_set"])
	}
}

// TestInjectStats_CacheHitAvoidsRepeatedQueries 缓存命中时不重复查询。
func TestInjectStats_CacheHitAvoidsRepeatedQueries(t *testing.T) {
	reader := &fakeStatsReader{
		rows: []usage.BreakdownRow{
			{ProviderCode: "openai", ModelAPIID: "gpt-4o", CallCount: 100, TotalTokens: 1000, TotalCostMicroUSD: 5000, SuccessRate: 0.9},
		},
	}
	inj := NewModelStatsInjector(reader, loggateway.NewNoop())
	inj.cacheTTL = 1 * time.Hour

	items := []ProviderModel{
		{Provider: "openai", Model: "gpt-4o", ConfigJSON: `{}`},
	}
	// 第一次调用：查询 DB
	inj.InjectStats(context.Background(), items)
	if reader.calls != 1 {
		t.Fatalf("first call: expected 1 DB query, got %d", reader.calls)
	}
	// 第二次调用：缓存命中，不查询
	inj.InjectStats(context.Background(), items)
	if reader.calls != 1 {
		t.Fatalf("second call: expected 1 DB query (cached), got %d", reader.calls)
	}
}

// TestInjectStats_ReaderErrorDoesNotModifyItems reader 返回错误时不修改 items。
func TestInjectStats_ReaderErrorDoesNotModifyItems(t *testing.T) {
	reader := &fakeStatsReader{
		err: context.DeadlineExceeded,
	}
	inj := NewModelStatsInjector(reader, loggateway.NewNoop())
	inj.cacheTTL = 1 * time.Hour

	original := `{"existing":"value"}`
	items := []ProviderModel{
		{Provider: "openai", Model: "gpt-4o", ConfigJSON: original},
	}
	inj.InjectStats(context.Background(), items)
	if items[0].ConfigJSON != original {
		t.Fatalf("expected config unchanged on error, got %s", items[0].ConfigJSON)
	}
}

// TestInjectStats_NilInjectorDoesNothing nil injector 不修改 items。
func TestInjectStats_NilInjectorDoesNothing(t *testing.T) {
	var inj *ModelStatsInjector
	items := []ProviderModel{
		{Provider: "openai", Model: "gpt-4o", ConfigJSON: `{"existing":"value"}`},
	}
	inj.InjectStats(context.Background(), items)
	if items[0].ConfigJSON != `{"existing":"value"}` {
		t.Fatalf("nil injector should not modify items, got %s", items[0].ConfigJSON)
	}
}

// TestInjectStats_ModelNotInStatsKeepConfigNull 模型不在统计列表中时保持统计字段为 null。
func TestInjectStats_ModelNotInStatsKeepConfigNull(t *testing.T) {
	reader := &fakeStatsReader{
		rows: []usage.BreakdownRow{
			{ProviderCode: "openai", ModelAPIID: "gpt-4o", CallCount: 100, TotalTokens: 1000, TotalCostMicroUSD: 5000, SuccessRate: 0.9},
		},
	}
	inj := NewModelStatsInjector(reader, loggateway.NewNoop())
	inj.cacheTTL = 1 * time.Hour

	items := []ProviderModel{
		{Provider: "anthropic", Model: "claude-3", ConfigJSON: `{"existing":"value"}`},
	}
	inj.InjectStats(context.Background(), items)

	var cfg map[string]any
	if err := json.Unmarshal([]byte(items[0].ConfigJSON), &cfg); err != nil {
		t.Fatalf("unmarshal config: %v", err)
	}
	// 不在统计列表中的模型不应该被注入统计字段
	if _, ok := cfg["usage_call_count_30d"]; ok {
		t.Fatalf("model not in stats should not have usage_call_count_30d")
	}
}
