package data

import (
	"context"
	"testing"

	"aranea-agents/internal/biz/session"
	"aranea-agents/internal/data/testhelper"
	"aranea-agents/pkg/loggateway"
)

// R4-Q10 回归：session_metrics 聚合管线。
// ① 首插必须应用 delta（此前 INSERT 全零、仅 ON CONFLICT 加 delta，首轮回丢失）；
// ② avg_latency_ms 滚动平均（样本基数 = run_count）；
// ③ token-only delta（无 run）不稀释 avg。
func TestUpsertSessionMetrics_FirstInsertAppliesDelta(t *testing.T) {
	ctx := context.Background()
	client, _ := testhelper.SetupTestPG(t)
	d := newDataFromClient(client, loggateway.NewNoop())
	repo := NewSessionMetricsRepo(d)

	if err := repo.ApplyMetricsDelta(ctx, &session.SessionMetricsDelta{
		SessionID: "s1", MessageCount: 2, RunCount: 1, ModelCallCount: 1,
		InputTokens: 100, OutputTokens: 20, TotalTokens: 120, LatencySumMs: 1500,
		LastMessageAt: "2026-08-30T00:00:00Z",
	}); err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	m, err := NewSessionMetricsReader(d).GetSessionMetrics(ctx, "s1")
	if err != nil || m == nil {
		t.Fatalf("read after first upsert: m=%v err=%v", m, err)
	}
	if m.MessageCount != 2 || m.RunCount != 1 || m.InputTokens != 100 {
		t.Fatalf("first insert lost delta: %+v", m)
	}
	if m.AvgLatencyMs != 1500 {
		t.Fatalf("avg after first run = %v, want 1500", m.AvgLatencyMs)
	}

	// 第二轮：计数累加 + avg 滚动 = (1500 + 2500) / 2。
	if err := repo.ApplyMetricsDelta(ctx, &session.SessionMetricsDelta{
		SessionID: "s1", MessageCount: 2, RunCount: 1, LatencySumMs: 2500,
	}); err != nil {
		t.Fatalf("second upsert: %v", err)
	}
	m, err = NewSessionMetricsReader(d).GetSessionMetrics(ctx, "s1")
	if err != nil {
		t.Fatalf("read after second upsert: %v", err)
	}
	if m.MessageCount != 4 || m.RunCount != 2 {
		t.Fatalf("accumulate wrong: msg=%d run=%d", m.MessageCount, m.RunCount)
	}
	if m.AvgLatencyMs != 2000 {
		t.Fatalf("avg after second run = %v, want 2000", m.AvgLatencyMs)
	}

	// token-only delta（usage 管线，无 run）：计数/token 累加，avg 不动。
	if err := repo.ApplyMetricsDelta(ctx, &session.SessionMetricsDelta{
		SessionID: "s1", ModelCallCount: 2, InputTokens: 500, OutputTokens: 50, TotalTokens: 550,
	}); err != nil {
		t.Fatalf("token-only upsert: %v", err)
	}
	m, _ = NewSessionMetricsReader(d).GetSessionMetrics(ctx, "s1")
	if m.RunCount != 2 || m.InputTokens != 600 || m.AvgLatencyMs != 2000 {
		t.Fatalf("token-only delta polluted avg: %+v", m)
	}
}

// R4-Q10 回归：context_used 无条件镜像进 session_metrics，max 取历史峰值。
func TestUpdateSessionContextFromLLMUsage_MirrorsMetrics(t *testing.T) {
	ctx := context.Background()
	client, _ := testhelper.SetupTestPG(t)
	d := newDataFromClient(client, loggateway.NewNoop())
	repo := NewSessionRepo(d, NewSessionMetricsRepo(d), nil)

	if _, err := client.Session.Create().
		SetID("s1").SetTitle("s1").SetStatus("active").Save(ctx); err != nil {
		t.Fatalf("create session: %v", err)
	}
	if err := repo.UpdateSessionContextFromLLMUsage(ctx, "s1", 40000, 100, 100000); err != nil {
		t.Fatalf("update context: %v", err)
	}
	m, err := NewSessionMetricsReader(d).GetSessionMetrics(ctx, "s1")
	if err != nil || m == nil {
		t.Fatalf("metrics row missing after context update: m=%v err=%v", m, err)
	}
	if m.ContextUsedTokens != 40000 || m.ContextUsedRatio != 0.4 || m.MaxContextUsedRatio != 0.4 {
		t.Fatalf("context not mirrored: %+v", m)
	}
	if m.ContextStatus == "" {
		t.Fatalf("context_status not mirrored")
	}

	// 水位回落：ratio 跟随当前值，max 保持峰值。
	if err := repo.UpdateSessionContextFromLLMUsage(ctx, "s1", 20000, 100, 100000); err != nil {
		t.Fatalf("update context 2: %v", err)
	}
	m, _ = NewSessionMetricsReader(d).GetSessionMetrics(ctx, "s1")
	if m.ContextUsedRatio != 0.2 || m.MaxContextUsedRatio != 0.4 {
		t.Fatalf("max peak lost: ratio=%v max=%v", m.ContextUsedRatio, m.MaxContextUsedRatio)
	}
}
