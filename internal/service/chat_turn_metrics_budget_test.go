package service

import (
	"context"
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
