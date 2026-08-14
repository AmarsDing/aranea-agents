package alert

import (
	"context"
	"sync"
	"testing"
	"time"

	"aranea-agents/pkg/loggateway"
)

// workerTestRepo implements the narrow alert ports (AlertRepo + EventCounter)
// the engine needs for worker tests.
type workerTestRepo struct {
	mu                   sync.Mutex
	listAlertRulesCalled bool
	countCalled          int
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

func TestAlertEvalWorker_Evaluate_CallsEngine(t *testing.T) {
	repo := &workerTestRepo{}
	engine := NewEngine(repo, repo, nil)
	rb := NewMetricRingBuffer()
	w := NewAlertEvalWorker(engine, rb, loggateway.NewNoop())
	w.ready.Store(true)
	w.evaluate(context.Background())
	repo.mu.Lock()
	wasCalled := repo.listAlertRulesCalled
	repo.mu.Unlock()
	if !wasCalled {
		t.Error("evaluate should call engine.EvaluateAlerts which calls ListAlertRules")
	}
}

func TestAlertEvalWorker_Evaluate_NotReady(t *testing.T) {
	repo := &workerTestRepo{}
	engine := NewEngine(repo, repo, nil)
	rb := NewMetricRingBuffer()
	w := NewAlertEvalWorker(engine, rb, loggateway.NewNoop())
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
	engine := NewEngine(repo, repo, nil)
	rb := NewMetricRingBuffer()
	w := NewAlertEvalWorker(engine, rb, loggateway.NewNoop())
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
	engine := NewEngine(repo, repo, nil)
	w := NewAlertEvalWorker(engine, nil, loggateway.NewNoop())
	w.rebuildFromDB(context.Background())
	time.Sleep(200 * time.Millisecond)
	if !w.ready.Load() {
		t.Error("rebuildFromDB should set ready to true even with nil buffer (logs warning)")
	}
}

func TestNewAlertEvalWorker_SetsInterval(t *testing.T) {
	t.Setenv("MONITOR_ALERT_EVAL_INTERVAL", "15s")
	engine := NewEngine(nil, nil, nil)
	rb := NewMetricRingBuffer()
	w := NewAlertEvalWorker(engine, rb, loggateway.NewNoop())
	if w.interval != 15*time.Second {
		t.Errorf("interval = %v, want %v", w.interval, 15*time.Second)
	}
}

func TestNewAlertEvalWorker_DefaultInterval(t *testing.T) {
	engine := NewEngine(nil, nil, nil)
	rb := NewMetricRingBuffer()
	w := NewAlertEvalWorker(engine, rb, loggateway.NewNoop())
	if w.interval != defaultEvalInterval {
		t.Errorf("interval = %v, want %v", w.interval, defaultEvalInterval)
	}
}
