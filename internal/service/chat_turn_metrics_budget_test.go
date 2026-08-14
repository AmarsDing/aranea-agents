package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"

	chatagent "aranea-agents/internal/agent"
	"aranea-agents/internal/metrics"
	"aranea-agents/pkg/loggateway"
)

func histogramSampleCount(t *testing.T, o prometheus.Observer) uint64 {
	t.Helper()
	metric, ok := o.(prometheus.Metric)
	if !ok {
		t.Fatalf("observer does not implement prometheus.Metric")
	}
	m := &dto.Metric{}
	if err := metric.Write(m); err != nil {
		t.Fatalf("write metric: %v", err)
	}
	return m.GetHistogram().GetSampleCount()
}

// context budget 台账在发进程日志的同时必须向
// aranea_context_budget_tokens{category} 直方图观测一次，
// 否则台账数据无法聚合分析（纯日志无闭环）。
func TestRecordContextBudgetLog_ObservesCategoryHistogram(t *testing.T) {
	m := &chatTurnMetrics{lg: loggateway.NewNoop()}
	ctx, _ := chatagent.WithContextBudget(context.Background())
	chatagent.RecordContextBudget(ctx, chatagent.ContextBudgetCategoryToolsSchema, 3500)

	h := metrics.ContextBudgetTokens.WithLabelValues(chatagent.ContextBudgetCategoryToolsSchema)
	before := histogramSampleCount(t, h)
	m.recordContextBudgetLog(ctx, TurnUsageParams{SessionID: "s1", RunID: "r1"})
	if after := histogramSampleCount(t, h); after-before != 1 {
		t.Fatalf("histogram sample delta: want 1, got %d", after-before)
	}
}

// 无台账挂载（非 chat 路径）或台账为空时必须 no-op，不产生观测。
func TestRecordContextBudgetLog_NoBudgetNoObservation(t *testing.T) {
	m := &chatTurnMetrics{lg: loggateway.NewNoop()}
	h := metrics.ContextBudgetTokens.WithLabelValues(chatagent.ContextBudgetCategoryHistory)
	before := histogramSampleCount(t, h)
	m.recordContextBudgetLog(context.Background(), TurnUsageParams{SessionID: "s1"})
	if after := histogramSampleCount(t, h); after != before {
		t.Fatalf("no-budget path must not observe, delta=%d", after-before)
	}
}

// S2：context budget 快照必须合并进 usage.metadata_json（context_budget 键），
// 否则台账只存在于进程日志/Prometheus，无法跨 turn 做 DB 侧聚合分析。
func TestMergeContextBudgetMetadata_MergesSnapshot(t *testing.T) {
	ctx, _ := chatagent.WithContextBudget(context.Background())
	chatagent.RecordContextBudget(ctx, chatagent.ContextBudgetCategoryToolsSchema, 3500)

	meta := mergeContextBudgetMetadata(ctx, `{"trace_id":"t1"}`)
	var payload map[string]any
	if err := json.Unmarshal([]byte(meta), &payload); err != nil {
		t.Fatalf("merged metadata not valid JSON: %v", err)
	}
	if payload["trace_id"] != "t1" {
		t.Fatalf("existing keys must be preserved, got %v", payload)
	}
	cb, ok := payload["context_budget"].(map[string]any)
	if !ok {
		t.Fatalf("context_budget key missing: %s", meta)
	}
	est, ok := cb["est_tokens"].(map[string]any)
	if !ok || est[chatagent.ContextBudgetCategoryToolsSchema] != float64(1000) {
		t.Fatalf("est_tokens.tools_schema: want 1000 (3500 chars / 3.5), got %v", cb["est_tokens"])
	}
	if cb["est_total_input"] != float64(1000) {
		t.Fatalf("est_total_input: want 1000, got %v", cb["est_total_input"])
	}
}

// 无台账或台账为空时原样透传，不污染 metadata。
func TestMergeContextBudgetMetadata_NoBudgetPassthrough(t *testing.T) {
	meta := `{"trace_id":"t1"}`
	if got := mergeContextBudgetMetadata(context.Background(), meta); got != meta {
		t.Fatalf("no-budget path must pass through, got %s", got)
	}
	ctx, _ := chatagent.WithContextBudget(context.Background())
	if got := mergeContextBudgetMetadata(ctx, meta); got != meta {
		t.Fatalf("empty-budget path must pass through, got %s", got)
	}
}
