package monitor

import (
	"context"
	"sync"
	"testing"
	"time"

	"aranea-agents/pkg/loggateway"
)

type workerTestRepo struct {
	Repo
	mu                 sync.Mutex
	listAlertRulesCalled bool
	countCalled        int
}

func (r *workerTestRepo) ListAuditLogs(ctx context.Context, query AuditQuery) (AuditListResult, error) {
	return AuditListResult{}, nil
}

func (r *workerTestRepo) InsertAuditLog(ctx context.Context, entry AuditLog) error {
	return nil
}

func (r *workerTestRepo) InsertMonitorEvent(ctx context.Context, ev EventWrite) error {
	return nil
}

func (r *workerTestRepo) ListMonitorEvents(ctx context.Context, query EventsQuery) (ListResult, error) {
	return ListResult{}, nil
}

func (r *workerTestRepo) GetMonitorEvent(ctx context.Context, id string) (PlatformRow, error) {
	return PlatformRow{}, nil
}

func (r *workerTestRepo) ListMonitorTraces(ctx context.Context, query TracesQuery) (ListResult, error) {
	return ListResult{}, nil
}

func (r *workerTestRepo) GetMonitorTrace(ctx context.Context, id string) (PlatformRow, error) {
	return PlatformRow{}, nil
}

func (r *workerTestRepo) ListAlertRules(ctx context.Context) ([]AlertRule, error) {
	r.mu.Lock()
	r.listAlertRulesCalled = true
	r.mu.Unlock()
	return nil, nil
}

func (r *workerTestRepo) ReplaceAlertRules(ctx context.Context, rules []AlertRule) error {
	return nil
}

func (r *workerTestRepo) UpdateAlertFiringState(ctx context.Context, id string, state AlertFiringState, lastFiredAt *time.Time, lastFiredValue float64, recoveredAt *time.Time) error {
	return nil
}

func (r *workerTestRepo) CountMonitorEventsSince(ctx context.Context, eventKey, status, sinceRFC3339, untilRFC3339 string) (int32, error) {
	r.mu.Lock()
	r.countCalled++
	r.mu.Unlock()
	return 5, nil
}

func (r *workerTestRepo) AvgRunnerCompletionDurationMsSince(ctx context.Context, sinceRFC3339 string) (float64, error) {
	return 0, nil
}

func (r *workerTestRepo) LatencyPercentilesSince(ctx context.Context, sinceRFC3339 string) (float64, float64, float64, error) {
	return 0, 0, 0, nil
}

func (r *workerTestRepo) ExistsRunnerCompletion(ctx context.Context, sessionID, invocationID string) (bool, error) {
	return false, nil
}

func (r *workerTestRepo) PatchRunnerCompletionMetadata(ctx context.Context, sessionID, runID, invocationID, patchJSON string) (bool, error) {
	return false, nil
}

func (r *workerTestRepo) InsertMonitorTrace(ctx context.Context, tw TraceWrite) error {
	return nil
}

func (r *workerTestRepo) UpsertMonitorTraceSpan(ctx context.Context, sw TraceSpanWrite) error {
	return nil
}

func (r *workerTestRepo) UpdateMonitorTraceCompletion(ctx context.Context, traceID string, status string, durationMs int64, spanCount, errorCount int, totalTokens int64, totalCostUsd float64) error {
	return nil
}

func (r *workerTestRepo) EnsureTraceSchema(ctx context.Context) error {
	return nil
}

func (r *workerTestRepo) ListRecentRunnerCompletions(ctx context.Context, since time.Duration, limit int) ([]RunnerCompletionRow, error) {
	return nil, nil
}

func TestEvalIntervalFromEnv_Default(t *testing.T) {
	got := evalIntervalFromEnv()
	if got != defaultEvalInterval {
		t.Errorf("evalIntervalFromEnv() = %v, want %v", got, defaultEvalInterval)
	}
}

func TestEvalIntervalFromEnv_ValidValue(t *testing.T) {
	t.Setenv("MONITOR_ALERT_EVAL_INTERVAL", "10s")
	got := evalIntervalFromEnv()
	if got != 10*time.Second {
		t.Errorf("evalIntervalFromEnv() = %v, want %v", got, 10*time.Second)
	}
}

func TestEvalIntervalFromEnv_BelowMinimum(t *testing.T) {
	t.Setenv("MONITOR_ALERT_EVAL_INTERVAL", "3s")
	got := evalIntervalFromEnv()
	if got != defaultEvalInterval {
		t.Errorf("evalIntervalFromEnv() = %v, want %v (below 5s minimum)", got, defaultEvalInterval)
	}
}

func TestEvalIntervalFromEnv_ExactMinimum(t *testing.T) {
	t.Setenv("MONITOR_ALERT_EVAL_INTERVAL", "5s")
	got := evalIntervalFromEnv()
	if got != 5*time.Second {
		t.Errorf("evalIntervalFromEnv() = %v, want %v", got, 5*time.Second)
	}
}

func TestEvalIntervalFromEnv_InvalidFormat(t *testing.T) {
	t.Setenv("MONITOR_ALERT_EVAL_INTERVAL", "not-a-duration")
	got := evalIntervalFromEnv()
	if got != defaultEvalInterval {
		t.Errorf("evalIntervalFromEnv() = %v, want %v (invalid format)", got, defaultEvalInterval)
	}
}

func TestEvalIntervalFromEnv_EmptyValue(t *testing.T) {
	t.Setenv("MONITOR_ALERT_EVAL_INTERVAL", "")
	got := evalIntervalFromEnv()
	if got != defaultEvalInterval {
		t.Errorf("evalIntervalFromEnv() = %v, want %v (empty value)", got, defaultEvalInterval)
	}
}

func TestEvalIntervalFromEnv_WhitespaceOnly(t *testing.T) {
	t.Setenv("MONITOR_ALERT_EVAL_INTERVAL", "   ")
	got := evalIntervalFromEnv()
	if got != defaultEvalInterval {
		t.Errorf("evalIntervalFromEnv() = %v, want %v (whitespace only)", got, defaultEvalInterval)
	}
}

func TestEvalIntervalFromEnv_MinutesFormat(t *testing.T) {
	t.Setenv("MONITOR_ALERT_EVAL_INTERVAL", "2m")
	got := evalIntervalFromEnv()
	if got != 2*time.Minute {
		t.Errorf("evalIntervalFromEnv() = %v, want %v", got, 2*time.Minute)
	}
}

func TestAlertEvalWorker_Evaluate_CallsUsecase(t *testing.T) {
	repo := &workerTestRepo{}
	uc := NewUsecase(repo, repo, repo, repo, repo, nil)
	rb := NewMetricRingBuffer()
	w := NewAlertEvalWorker(uc, rb, loggateway.Global())
	w.ready.Store(true)
	w.evaluate(context.Background())
	repo.mu.Lock()
	wasCalled := repo.listAlertRulesCalled
	repo.mu.Unlock()
	if !wasCalled {
		t.Error("evaluate should call usecase.EvaluateAlerts which calls ListAlertRules")
	}
}

func TestAlertEvalWorker_Evaluate_NotReady(t *testing.T) {
	repo := &workerTestRepo{}
	uc := NewUsecase(repo, repo, repo, repo, repo, nil)
	rb := NewMetricRingBuffer()
	w := NewAlertEvalWorker(uc, rb, loggateway.Global())
	w.ready.Store(false)
	w.evaluate(context.Background())
	repo.mu.Lock()
	wasCalled := repo.listAlertRulesCalled
	repo.mu.Unlock()
	if wasCalled {
		t.Error("evaluate should not call EvaluateAlerts when not ready")
	}
}

func TestAlertEvalWorker_RebuildFromDB_SetsReady(t *testing.T) {
	repo := &workerTestRepo{}
	uc := NewUsecase(repo, repo, repo, repo, repo, nil)
	rb := NewMetricRingBuffer()
	w := NewAlertEvalWorker(uc, rb, loggateway.Global())
	w.rebuildFromDB(context.Background())
	time.Sleep(200 * time.Millisecond)
	if !w.ready.Load() {
		t.Error("rebuildFromDB should set ready to true")
	}
	repo.mu.Lock()
	called := repo.countCalled > 0
	repo.mu.Unlock()
	if !called {
		t.Error("rebuildFromDB should call RebuildRingBuffer which calls CountMonitorEventsSince")
	}
}

func TestAlertEvalWorker_RebuildFromDB_NilBuffer(t *testing.T) {
	repo := &workerTestRepo{}
	uc := NewUsecase(repo, repo, repo, repo, repo, nil)
	w := NewAlertEvalWorker(uc, nil, loggateway.Global())
	w.rebuildFromDB(context.Background())
	time.Sleep(200 * time.Millisecond)
	if !w.ready.Load() {
		t.Error("rebuildFromDB should set ready to true even with nil buffer (logs warning)")
	}
}

func TestNewAlertEvalWorker_SetsInterval(t *testing.T) {
	t.Setenv("MONITOR_ALERT_EVAL_INTERVAL", "15s")
	uc := NewUsecase(nil, nil, nil, nil, nil, nil)
	rb := NewMetricRingBuffer()
	w := NewAlertEvalWorker(uc, rb, loggateway.Global())
	if w.interval != 15*time.Second {
		t.Errorf("interval = %v, want %v", w.interval, 15*time.Second)
	}
}

func TestNewAlertEvalWorker_DefaultInterval(t *testing.T) {
	uc := NewUsecase(nil, nil, nil, nil, nil, nil)
	rb := NewMetricRingBuffer()
	w := NewAlertEvalWorker(uc, rb, loggateway.Global())
	if w.interval != defaultEvalInterval {
		t.Errorf("interval = %v, want %v", w.interval, defaultEvalInterval)
	}
}
