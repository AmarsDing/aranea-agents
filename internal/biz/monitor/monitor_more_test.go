package monitor_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"aranea-agents/internal/biz/monitor"
	"aranea-agents/internal/event/contract"
	"aranea-agents/pkg/loggateway"
)

type mockBus struct {
	ch       chan contract.Envelope
	unsub    func()
	dropCnt  uint64
}

func newMockBus() *mockBus {
	return &mockBus{
		ch: make(chan contract.Envelope, 64),
	}
}

func (m *mockBus) Publish(_ context.Context, env contract.Envelope) {
	select {
	case m.ch <- env:
	default:
		m.dropCnt++
	}
}

func (m *mockBus) Subscribe(_ contract.SubscribeOptions) (<-chan contract.Envelope, func()) {
	return m.ch, func() {}
}

func (m *mockBus) DropCount() uint64 { return m.dropCnt }

func TestAlertEvalWorker_OnCompletion_Success(t *testing.T) {
	uc := monitor.NewUsecase(nil, nil)
	rb := monitor.NewMetricRingBuffer()
	w := monitor.NewAlertEvalWorker(uc, rb)
	w.OnCompletion("success", 150)
}

func TestAlertEvalWorker_OnCompletion_Error(t *testing.T) {
	uc := monitor.NewUsecase(nil, nil)
	rb := monitor.NewMetricRingBuffer()
	w := monitor.NewAlertEvalWorker(uc, rb)
	w.OnCompletion("error", 300)
}

func TestAlertEvalWorker_OnCompletion_NilWorker(t *testing.T) {
	var w *monitor.AlertEvalWorker
	w.OnCompletion("success", 100)
}

func TestAlertEvalWorker_OnCompletion_NilBuffer(t *testing.T) {
	uc := monitor.NewUsecase(nil, nil)
	w := monitor.NewAlertEvalWorker(uc, nil)
	w.OnCompletion("success", 100)
}

func TestAlertEvalWorker_Ready_BeforeStart(t *testing.T) {
	uc := monitor.NewUsecase(nil, nil)
	rb := monitor.NewMetricRingBuffer()
	w := monitor.NewAlertEvalWorker(uc, rb)
	if w.Ready() {
		t.Error("expected Ready() = false before Start")
	}
}

func TestAlertEvalWorker_Ready_NilWorker(t *testing.T) {
	var w *monitor.AlertEvalWorker
	if w.Ready() {
		t.Error("nil worker Ready() should return false")
	}
}

func TestAlertEvalWorker_NilUsecase(t *testing.T) {
	w := monitor.NewAlertEvalWorker(nil, monitor.NewMetricRingBuffer())
	if w != nil {
		t.Error("NewAlertEvalWorker(nil, ...) should return nil")
	}
}

func newTestTraceProjector(repo monitor.Repo) *monitor.TraceProjector {
	return monitor.NewTraceProjector(repo, loggateway.Global(), newMockBus())
}

func TestTraceProjector_OnRunnerCompletion_Success(t *testing.T) {
	called := false
	repo := &mockRepo{
		updateMonitorTraceCompletionFn: func(_ context.Context, traceID, status string, durationMs int64, spanCount, errorCount int, totalTokens int64, totalCostUsd float64) error {
			called = true
			if traceID != "trace-1" {
				t.Errorf("traceID = %q, want %q", traceID, "trace-1")
			}
			if status != "success" {
				t.Errorf("status = %q, want %q", status, "success")
			}
			if durationMs != 500 {
				t.Errorf("durationMs = %d, want 500", durationMs)
			}
			return nil
		},
	}
	p := newTestTraceProjector(repo)
	p.AddTestTrace("trace-1", time.Now())
	p.OnRunnerCompletion(context.Background(), "trace-1", "success", 500)
	if !called {
		t.Error("UpdateMonitorTraceCompletion not called")
	}
}

func TestTraceProjector_OnRunnerCompletion_RepoError(t *testing.T) {
	repo := &mockRepo{
		updateMonitorTraceCompletionFn: func(context.Context, string, string, int64, int, int, int64, float64) error {
			return fmt.Errorf("db error")
		},
	}
	p := newTestTraceProjector(repo)
	p.AddTestTrace("trace-1", time.Now())
	p.OnRunnerCompletion(context.Background(), "trace-1", "error", 100)
}

func TestTraceProjector_OnRunnerCompletion_EmptyTraceID(t *testing.T) {
	called := false
	repo := &mockRepo{
		updateMonitorTraceCompletionFn: func(context.Context, string, string, int64, int, int, int64, float64) error {
			called = true
			return nil
		},
	}
	p := newTestTraceProjector(repo)
	p.OnRunnerCompletion(context.Background(), "", "success", 500)
	if called {
		t.Error("UpdateMonitorTraceCompletion should not be called for empty traceID")
	}
}

func TestTraceProjector_OnRunnerCompletion_NilProjector(t *testing.T) {
	var p *monitor.TraceProjector
	p.OnRunnerCompletion(context.Background(), "trace-1", "success", 500)
}

func TestTraceProjector_OnRunnerCompletion_ErrorStatusSetsErrCount(t *testing.T) {
	var gotErrCount int
	repo := &mockRepo{
		updateMonitorTraceCompletionFn: func(_ context.Context, _ string, _ string, _ int64, _ int, errorCount int, _ int64, _ float64) error {
			gotErrCount = errorCount
			return nil
		},
	}
	p := newTestTraceProjector(repo)
	p.AddTestTrace("trace-1", time.Now())
	p.OnRunnerCompletion(context.Background(), "trace-1", "error", 100)
	if gotErrCount < 1 {
		t.Errorf("errorCount = %d, want >= 1 for error status", gotErrCount)
	}
}

func TestTraceProjector_OnRunnerCompletion_RemovesFromMap(t *testing.T) {
	repo := &mockRepo{}
	p := newTestTraceProjector(repo)
	p.AddTestTrace("trace-1", time.Now())
	if p.TraceCount() != 1 {
		t.Errorf("TraceCount() = %d, want 1 before completion", p.TraceCount())
	}
	p.OnRunnerCompletion(context.Background(), "trace-1", "success", 500)
	if p.TraceCount() != 0 {
		t.Errorf("TraceCount() = %d, want 0 after completion", p.TraceCount())
	}
}

func TestTraceProjector_EnsureTrace_NewTraceCreated(t *testing.T) {
	called := false
	repo := &mockRepo{
		insertMonitorTraceFn: func(_ context.Context, tw monitor.TraceWrite) error {
			called = true
			if tw.TraceID != "trace-new" {
				t.Errorf("TraceID = %q, want %q", tw.TraceID, "trace-new")
			}
			if tw.Status != "running" {
				t.Errorf("Status = %q, want %q", tw.Status, "running")
			}
			return nil
		},
	}
	p := newTestTraceProjector(repo)
	p.EnsureTraceExposed(context.Background(), "trace-new", "sess-1", "run-1", "agent-1", "team-1", "domain-1")
	if !called {
		t.Error("InsertMonitorTrace not called for new trace")
	}
	if p.TraceCount() != 1 {
		t.Errorf("TraceCount() = %d, want 1 after ensureTrace", p.TraceCount())
	}
}

func TestTraceProjector_EnsureTrace_ExistingTraceReused(t *testing.T) {
	called := false
	repo := &mockRepo{
		insertMonitorTraceFn: func(_ context.Context, tw monitor.TraceWrite) error {
			called = true
			return nil
		},
	}
	p := newTestTraceProjector(repo)
	p.EnsureTraceExposed(context.Background(), "trace-1", "sess-1", "run-1", "agent-1", "team-1", "domain-1")
	if !called {
		t.Error("InsertMonitorTrace should be called on first ensureTrace")
	}
	called = false
	p.EnsureTraceExposed(context.Background(), "trace-1", "sess-1", "run-1", "agent-1", "team-1", "domain-1")
	if called {
		t.Error("InsertMonitorTrace should not be called for existing trace")
	}
	if p.TraceCount() != 1 {
		t.Errorf("TraceCount() = %d, want 1 (no duplicate)", p.TraceCount())
	}
}

func TestTraceProjector_EnsureTrace_InsertError(t *testing.T) {
	repo := &mockRepo{
		insertMonitorTraceFn: func(context.Context, monitor.TraceWrite) error {
			return fmt.Errorf("insert error")
		},
	}
	p := newTestTraceProjector(repo)
	p.EnsureTraceExposed(context.Background(), "trace-1", "sess-1", "run-1", "agent-1", "team-1", "domain-1")
	if p.TraceCount() != 1 {
		t.Errorf("TraceCount() = %d, want 1 (trace still added to memory map even if insert fails)", p.TraceCount())
	}
}

func TestTraceProjector_EvictStaleTraces_StaleEvicted(t *testing.T) {
	repo := &mockRepo{}
	p := newTestTraceProjector(repo)
	p.AddTestTrace("stale-1", time.Now().Add(-15*time.Minute))
	p.AddTestTrace("stale-2", time.Now().Add(-20*time.Minute))
	if p.TraceCount() != 2 {
		t.Fatalf("TraceCount() = %d, want 2 before eviction", p.TraceCount())
	}
	p.EvictStaleTracesExposed()
	if p.TraceCount() != 0 {
		t.Errorf("TraceCount() = %d, want 0 after evicting stale traces", p.TraceCount())
	}
}

func TestTraceProjector_EvictStaleTraces_RecentKept(t *testing.T) {
	repo := &mockRepo{}
	p := newTestTraceProjector(repo)
	p.AddTestTrace("recent-1", time.Now().Add(-1*time.Minute))
	p.AddTestTrace("recent-2", time.Now().Add(-5*time.Minute))
	p.AddTestTrace("stale-1", time.Now().Add(-15*time.Minute))
	p.EvictStaleTracesExposed()
	if p.TraceCount() != 2 {
		t.Errorf("TraceCount() = %d, want 2 (recent traces kept)", p.TraceCount())
	}
}

func TestTraceProjector_EvictStaleTraces_EmptyMap(t *testing.T) {
	repo := &mockRepo{}
	p := newTestTraceProjector(repo)
	p.EvictStaleTracesExposed()
	if p.TraceCount() != 0 {
		t.Errorf("TraceCount() = %d, want 0 for empty map", p.TraceCount())
	}
}

func TestTraceProjector_NewTraceProjector_NilRepo(t *testing.T) {
	p := monitor.NewTraceProjector(nil, loggateway.Global(), newMockBus())
	if p != nil {
		t.Error("NewTraceProjector(nil, bus) should return nil")
	}
}

func TestTraceProjector_NewTraceProjector_NilBus(t *testing.T) {
	repo := &mockRepo{}
	p := monitor.NewTraceProjector(repo, loggateway.Global())
	if p != nil {
		t.Error("NewTraceProjector(repo) with no buses should return nil")
	}
}

func TestTraceProjector_NewTraceProjector_DuplicateBus(t *testing.T) {
	repo := &mockRepo{}
	bus := newMockBus()
	p := monitor.NewTraceProjector(repo, loggateway.Global(), bus, bus)
	if p == nil {
		t.Fatal("NewTraceProjector with duplicate bus should still return non-nil")
	}
}
