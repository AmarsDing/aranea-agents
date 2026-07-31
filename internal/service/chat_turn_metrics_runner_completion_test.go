package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// runnerCompletionMockRepo satisfies the 5 monitor repo interfaces used by
// biz.NewMonitorUsecase; only the runner.completion write path is recorded.
type runnerCompletionMockRepo struct {
	inserted []biz.MonitorEventWrite
	exists   bool
}

func (r *runnerCompletionMockRepo) ListAuditLogs(context.Context, biz.AuditQuery) (biz.AuditListResult, error) {
	return biz.AuditListResult{}, nil
}
func (r *runnerCompletionMockRepo) InsertAuditLog(context.Context, biz.AuditLog) error { return nil }
func (r *runnerCompletionMockRepo) DeleteAuditLogs(context.Context) (int, error) {
	return 0, nil
}
func (r *runnerCompletionMockRepo) InsertMonitorEvent(_ context.Context, ev biz.MonitorEventWrite) error {
	r.inserted = append(r.inserted, ev)
	return nil
}
func (r *runnerCompletionMockRepo) ListMonitorEvents(context.Context, biz.MonitorEventsQuery) (biz.MonitorListResult, error) {
	return biz.MonitorListResult{}, nil
}
func (r *runnerCompletionMockRepo) GetMonitorEvent(context.Context, string) (biz.MonitorPlatformRow, error) {
	return biz.MonitorPlatformRow{}, nil
}
func (r *runnerCompletionMockRepo) CountMonitorEventsSince(context.Context, string, string, string, string) (int32, error) {
	return 0, nil
}
func (r *runnerCompletionMockRepo) ListMonitorTraces(context.Context, biz.MonitorTracesQuery) (biz.MonitorListResult, error) {
	return biz.MonitorListResult{}, nil
}
func (r *runnerCompletionMockRepo) GetMonitorTrace(context.Context, string) (biz.MonitorPlatformRow, error) {
	return biz.MonitorPlatformRow{}, nil
}
func (r *runnerCompletionMockRepo) InsertMonitorTrace(context.Context, biz.MonitorTraceWrite) error {
	return nil
}
func (r *runnerCompletionMockRepo) UpsertMonitorTraceSpan(context.Context, biz.MonitorTraceSpanWrite) error {
	return nil
}
func (r *runnerCompletionMockRepo) UpdateMonitorTraceCompletion(context.Context, string, biz.MonitorTraceCompletion) error {
	return nil
}
func (r *runnerCompletionMockRepo) InterruptStaleTraces(context.Context, time.Time) (int64, error) {
	return 0, nil
}
func (r *runnerCompletionMockRepo) DeleteMonitorEventsOlderThan(context.Context, time.Time) (int, error) {
	return 0, nil
}
func (r *runnerCompletionMockRepo) EnsureTraceSchema(context.Context) error { return nil }
func (r *runnerCompletionMockRepo) ListAlertRules(context.Context) ([]biz.MonitorAlertRule, error) {
	return nil, nil
}
func (r *runnerCompletionMockRepo) ReplaceAlertRules(context.Context, []biz.MonitorAlertRule) error {
	return nil
}
func (r *runnerCompletionMockRepo) UpdateAlertFiringState(context.Context, string, biz.MonitorAlertFiringState, *time.Time, float64, *time.Time) error {
	return nil
}
func (r *runnerCompletionMockRepo) ExistsRunnerCompletion(context.Context, string, string) (bool, error) {
	return r.exists, nil
}
func (r *runnerCompletionMockRepo) PatchRunnerCompletionMetadata(context.Context, string, string, string, string) (bool, error) {
	return false, nil
}
func (r *runnerCompletionMockRepo) AvgRunnerCompletionDurationMsSince(context.Context, string) (float64, error) {
	return 0, nil
}
func (r *runnerCompletionMockRepo) LatencyPercentilesSince(context.Context, string) (float64, float64, float64, error) {
	return 0, 0, 0, nil
}
func (r *runnerCompletionMockRepo) ListRecentRunnerCompletions(context.Context, time.Duration, int) ([]biz.RunnerCompletionRow, error) {
	return nil, nil
}

func newRunnerCompletionTestMetrics(repo *runnerCompletionMockRepo) *chatTurnMetrics {
	uc := biz.NewMonitorUsecase(repo, repo, repo, repo, repo, nil)
	return &chatTurnMetrics{monitor: uc, lg: loggateway.NewNoop()}
}

// F1 回归：chat turn 终态必须写 runner.completion 监控事件（Runner 指标 +
// runner.error_rate 告警的数据源）。2026-06-26 Activity-First 迁移删掉了
// 唯一生产写入方（event_bus_runner_handler.go），本测试锁定恢复后的行为。
func TestChatTurnMetrics_RecordRunnerCompletion_OK(t *testing.T) {
	repo := &runnerCompletionMockRepo{}
	tm := newRunnerCompletionTestMetrics(repo)

	p := TurnUsageParams{
		SessionID: "sess-1",
		RunID:     "run-1",
		AgentID:   "ag-1",
		AgentKey:  "demo",
		Status:    "ok",
		Latency:   1500 * time.Millisecond,
	}
	tm.recordRunnerCompletion(context.Background(), p, "tr-1")

	if len(repo.inserted) != 1 {
		t.Fatalf("inserted=%d want 1", len(repo.inserted))
	}
	ev := repo.inserted[0]
	if ev.EventKey != "runner.completion" {
		t.Fatalf("event_key=%q", ev.EventKey)
	}
	if ev.Status != "ok" {
		t.Fatalf("status=%q want ok", ev.Status)
	}
	var meta map[string]any
	if err := json.Unmarshal([]byte(ev.MetadataJSON), &meta); err != nil {
		t.Fatal(err)
	}
	if meta["session_id"] != "sess-1" || meta["run_id"] != "run-1" || meta["trace_id"] != "tr-1" {
		t.Fatalf("correlation fields: %+v", meta)
	}
	if meta["run_kind"] != "chat" {
		t.Fatalf("run_kind=%v want chat", meta["run_kind"])
	}
	if meta["duration_ms"].(float64) != 1500 {
		t.Fatalf("duration_ms=%v want 1500", meta["duration_ms"])
	}
}

func TestChatTurnMetrics_RecordRunnerCompletion_Error(t *testing.T) {
	repo := &runnerCompletionMockRepo{}
	tm := newRunnerCompletionTestMetrics(repo)

	p := TurnUsageParams{
		SessionID: "sess-1",
		RunID:     "run-1",
		AgentID:   "ag-1",
		Status:    "error",
		Latency:   300 * time.Millisecond,
		ErrMsg:    "provider timeout",
	}
	tm.recordRunnerCompletion(context.Background(), p, "")

	if len(repo.inserted) != 1 {
		t.Fatalf("inserted=%d want 1", len(repo.inserted))
	}
	ev := repo.inserted[0]
	if ev.Status != "error" {
		t.Fatalf("status=%q want error", ev.Status)
	}
	var meta map[string]any
	if err := json.Unmarshal([]byte(ev.MetadataJSON), &meta); err != nil {
		t.Fatal(err)
	}
	errObj, ok := meta["error"].(map[string]any)
	if !ok || errObj["message"] != "provider timeout" {
		t.Fatalf("error meta: %+v", meta)
	}
}

// timeout_degraded 等非 error 终态按 ok 记录（运行实际产出了回复）。
func TestChatTurnMetrics_RecordRunnerCompletion_DegradedCountsAsOK(t *testing.T) {
	repo := &runnerCompletionMockRepo{}
	tm := newRunnerCompletionTestMetrics(repo)

	p := TurnUsageParams{SessionID: "sess-1", RunID: "run-1", Status: "timeout_degraded", Latency: 100 * time.Millisecond}
	tm.recordRunnerCompletion(context.Background(), p, "")

	if len(repo.inserted) != 1 || repo.inserted[0].Status != "ok" {
		t.Fatalf("inserted=%+v", repo.inserted)
	}
}

func TestChatTurnMetrics_RecordRunnerCompletion_NilMonitor(t *testing.T) {
	tm := &chatTurnMetrics{lg: loggateway.NewNoop()}
	p := TurnUsageParams{SessionID: "sess-1", RunID: "run-1", Status: "ok"}
	// 不应 panic
	tm.recordRunnerCompletion(context.Background(), p, "")
}

// 幂等：同 (sessionID, runID) 已存在时不重复插入。
func TestChatTurnMetrics_RecordRunnerCompletion_Idempotent(t *testing.T) {
	repo := &runnerCompletionMockRepo{exists: true}
	tm := newRunnerCompletionTestMetrics(repo)

	p := TurnUsageParams{SessionID: "sess-1", RunID: "run-1", Status: "ok", Latency: 100 * time.Millisecond}
	tm.recordRunnerCompletion(context.Background(), p, "")

	if len(repo.inserted) != 0 {
		t.Fatalf("inserted=%d want 0 (idempotent skip)", len(repo.inserted))
	}
}
