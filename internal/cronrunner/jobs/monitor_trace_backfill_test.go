package jobs_test

import (
	"context"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/monitor"
	"aranea-agents/internal/cronrunner/jobs"
	"aranea-agents/pkg/loggateway"
)

type backfillTraceRepo struct {
	inserted   []biz.MonitorTraceWrite
	completed  []monitor.TraceCompletion
	sweptFor   *time.Time
	ensureFail bool
}

func (m *backfillTraceRepo) EnsureTraceSchema(context.Context) error {
	if m.ensureFail {
		return errTest
	}
	return nil
}

var errTest = &testError{}

type testError struct{}

func (*testError) Error() string { return "test error" }

func (m *backfillTraceRepo) InsertMonitorTrace(_ context.Context, tw biz.MonitorTraceWrite) error {
	m.inserted = append(m.inserted, tw)
	return nil
}

func (m *backfillTraceRepo) UpdateMonitorTraceCompletion(_ context.Context, _ string, c monitor.TraceCompletion) error {
	m.completed = append(m.completed, c)
	return nil
}

func (m *backfillTraceRepo) InterruptStaleTraces(_ context.Context, olderThan time.Time) (int64, error) {
	m.sweptFor = &olderThan
	return 2, nil
}

// Unused TraceRepo methods — the worker must not call them.
func (m *backfillTraceRepo) ListMonitorTraces(context.Context, biz.MonitorTracesQuery) (biz.MonitorListResult, error) {
	return biz.MonitorListResult{}, nil
}
func (m *backfillTraceRepo) GetMonitorTrace(context.Context, string) (biz.MonitorPlatformRow, error) {
	return biz.MonitorPlatformRow{}, nil
}
func (m *backfillTraceRepo) UpsertMonitorTraceSpan(context.Context, biz.MonitorTraceSpanWrite) error {
	return nil
}

type backfillCompletionRepo struct {
	rows []biz.RunnerCompletionRow
}

func (m *backfillCompletionRepo) ListRecentRunnerCompletions(context.Context, time.Duration, int) ([]biz.RunnerCompletionRow, error) {
	return m.rows, nil
}
func (m *backfillCompletionRepo) ExistsRunnerCompletion(context.Context, string, string) (bool, error) {
	return false, nil
}
func (m *backfillCompletionRepo) PatchRunnerCompletionMetadata(context.Context, string, string, string, string) (bool, error) {
	return false, nil
}
func (m *backfillCompletionRepo) AvgRunnerCompletionDurationMsSince(context.Context, string) (float64, error) {
	return 0, nil
}
func (m *backfillCompletionRepo) LatencyPercentilesSince(context.Context, string) (float64, float64, float64, error) {
	return 0, 0, 0, nil
}

type backfillUsageRepo struct {
	aggs map[string]monitor.UsageAggregate
}

func (m *backfillUsageRepo) AggregateUsageByTrace(_ context.Context, traceID string) (monitor.UsageAggregate, error) {
	return m.aggs[traceID], nil
}

// Backfill must aggregate tokens/cost/provider/model from usage events instead
// of hardcoding zeros, and must sweep stale running traces to interrupted.
func TestMonitorTraceBackfill_RunOnce_AggregatesUsageAndSweeps(t *testing.T) {
	traceRepo := &backfillTraceRepo{}
	completionRepo := &backfillCompletionRepo{
		rows: []biz.RunnerCompletionRow{
			{
				TraceID:    "trace-1",
				SessionID:  "sess-1",
				RunID:      "run-1",
				AgentID:    "agent-1",
				Status:     "ok",
				DurationMs: 1500,
				CreatedAt:  time.Now().UTC().Format(time.RFC3339),
			},
		},
	}
	usageRepo := &backfillUsageRepo{
		aggs: map[string]monitor.UsageAggregate{
			"trace-1": {
				TotalTokens:  2048,
				TotalCostUsd: 0.0123,
				Provider:     "deepseek",
				Model:        "deepseek-chat",
				CallCount:    3,
			},
		},
	}

	w := jobs.NewMonitorTraceBackfillWorker(traceRepo, completionRepo, usageRepo, loggateway.NewNoop())
	w.RunOnceExposed(context.Background())

	if len(traceRepo.inserted) != 1 {
		t.Fatalf("InsertMonitorTrace calls = %d, want 1", len(traceRepo.inserted))
	}
	if traceRepo.inserted[0].Provider != "deepseek" || traceRepo.inserted[0].Model != "deepseek-chat" {
		t.Errorf("inserted provider/model = %q/%q, want deepseek/deepseek-chat",
			traceRepo.inserted[0].Provider, traceRepo.inserted[0].Model)
	}
	if len(traceRepo.completed) != 1 {
		t.Fatalf("UpdateMonitorTraceCompletion calls = %d, want 1", len(traceRepo.completed))
	}
	c := traceRepo.completed[0]
	if c.TotalTokens != 2048 {
		t.Errorf("TotalTokens = %d, want 2048", c.TotalTokens)
	}
	if c.TotalCostUsd != 0.0123 {
		t.Errorf("TotalCostUsd = %v, want 0.0123", c.TotalCostUsd)
	}
	if c.Provider != "deepseek" || c.Model != "deepseek-chat" {
		t.Errorf("completion provider/model = %q/%q, want deepseek/deepseek-chat", c.Provider, c.Model)
	}
	if traceRepo.sweptFor == nil {
		t.Error("InterruptStaleTraces not called")
	}
}
