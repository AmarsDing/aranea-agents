package trace_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"aranea-agents/internal/biz/monitor/trace"
	"aranea-agents/pkg/loggateway"
)

func newTestTraceProjector(repo trace.Writer) *trace.TraceProjector {
	return trace.NewTraceProjector(repo, loggateway.NewNoop(), nil, &stubMonitorBus{})
}

func TestTraceProjector_OnRunnerCompletion_Success(t *testing.T) {
	called := false
	repo := &mockTraceWriter{
		updateMonitorTraceCompletionFn: func(_ context.Context, traceID string, c trace.TraceCompletion) error {
			called = true
			if traceID != "trace-1" {
				t.Errorf("traceID = %q, want %q", traceID, "trace-1")
			}
			if c.Status != "success" {
				t.Errorf("status = %q, want %q", c.Status, "success")
			}
			if c.DurationMs != 500 {
				t.Errorf("durationMs = %d, want 500", c.DurationMs)
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
	repo := &mockTraceWriter{
		updateMonitorTraceCompletionFn: func(context.Context, string, trace.TraceCompletion) error {
			return fmt.Errorf("db error")
		},
	}
	p := newTestTraceProjector(repo)
	p.AddTestTrace("trace-1", time.Now())
	p.OnRunnerCompletion(context.Background(), "trace-1", "error", 100)
}

func TestTraceProjector_OnRunnerCompletion_EmptyTraceID(t *testing.T) {
	called := false
	repo := &mockTraceWriter{
		updateMonitorTraceCompletionFn: func(context.Context, string, trace.TraceCompletion) error {
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
	var p *trace.TraceProjector
	p.OnRunnerCompletion(context.Background(), "trace-1", "success", 500)
}

func TestTraceProjector_OnRunnerCompletion_ErrorStatusSetsErrCount(t *testing.T) {
	var gotErrCount int
	repo := &mockTraceWriter{
		updateMonitorTraceCompletionFn: func(_ context.Context, _ string, c trace.TraceCompletion) error {
			gotErrCount = c.ErrorCount
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
	repo := &mockTraceWriter{}
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
	repo := &mockTraceWriter{
		insertMonitorTraceFn: func(_ context.Context, tw trace.TraceWrite) error {
			called = true
			if tw.TraceID != "trace-new" {
				t.Errorf("TraceID = %q, want %q", tw.TraceID, "trace-new")
			}
			if tw.Status != "running" {
				t.Errorf("Status = %q, want %q", tw.Status, "running")
			}
			if tw.Provider != "openai" {
				t.Errorf("Provider = %q, want %q", tw.Provider, "openai")
			}
			if tw.Model != "gpt-4o" {
				t.Errorf("Model = %q, want %q", tw.Model, "gpt-4o")
			}
			return nil
		},
	}
	p := newTestTraceProjector(repo)
	p.EnsureTraceExposed(context.Background(), "trace-new", "sess-1", "run-1", "agent-1", "openai", "gpt-4o", "team-1", "domain-1")
	if !called {
		t.Error("InsertMonitorTrace not called for new trace")
	}
	if p.TraceCount() != 1 {
		t.Errorf("TraceCount() = %d, want 1 after ensureTrace", p.TraceCount())
	}
}

func TestTraceProjector_EnsureTrace_ExistingTraceReused(t *testing.T) {
	called := false
	repo := &mockTraceWriter{
		insertMonitorTraceFn: func(_ context.Context, tw trace.TraceWrite) error {
			called = true
			return nil
		},
	}
	p := newTestTraceProjector(repo)
	p.EnsureTraceExposed(context.Background(), "trace-1", "sess-1", "run-1", "agent-1", "openai", "gpt-4o", "team-1", "domain-1")
	if !called {
		t.Error("InsertMonitorTrace should be called on first ensureTrace")
	}
	called = false
	p.EnsureTraceExposed(context.Background(), "trace-1", "sess-1", "run-1", "agent-1", "openai", "gpt-4o", "team-1", "domain-1")
	if called {
		t.Error("InsertMonitorTrace should not be called for existing trace")
	}
	if p.TraceCount() != 1 {
		t.Errorf("TraceCount() = %d, want 1 (no duplicate)", p.TraceCount())
	}
}

func TestTraceProjector_EnsureTrace_InsertError(t *testing.T) {
	repo := &mockTraceWriter{
		insertMonitorTraceFn: func(context.Context, trace.TraceWrite) error {
			return fmt.Errorf("insert error")
		},
	}
	p := newTestTraceProjector(repo)
	p.EnsureTraceExposed(context.Background(), "trace-1", "sess-1", "run-1", "agent-1", "openai", "gpt-4o", "team-1", "domain-1")
	if p.TraceCount() != 1 {
		t.Errorf("TraceCount() = %d, want 1 (trace still added to memory map even if insert fails)", p.TraceCount())
	}
}

func TestTraceProjector_EvictStaleTraces_StaleEvicted(t *testing.T) {
	repo := &mockTraceWriter{}
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
	repo := &mockTraceWriter{}
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
	repo := &mockTraceWriter{}
	p := newTestTraceProjector(repo)
	p.EvictStaleTracesExposed()
	if p.TraceCount() != 0 {
		t.Errorf("TraceCount() = %d, want 0 for empty map", p.TraceCount())
	}
}

func TestTraceProjector_NewTraceProjector_NilRepo(t *testing.T) {
	p := trace.NewTraceProjector(nil, loggateway.NewNoop(), nil, &stubMonitorBus{})
	if p != nil {
		t.Error("NewTraceProjector(nil, bus) should return nil")
	}
}

func TestTraceProjector_NewTraceProjector_NilBus(t *testing.T) {
	repo := &mockTraceWriter{}
	p := trace.NewTraceProjector(repo, loggateway.NewNoop(), nil)
	if p != nil {
		t.Error("NewTraceProjector(repo) with no buses should return nil")
	}
}

func TestTraceProjector_NewTraceProjector_DuplicateBus(t *testing.T) {
	repo := &mockTraceWriter{}
	bus := &stubMonitorBus{}
	p := trace.NewTraceProjector(repo, loggateway.NewNoop(), nil, bus, bus)
	if p == nil {
		t.Fatal("NewTraceProjector with duplicate bus should still return non-nil")
	}
}

// The next three tests cover the activity signals added to support the
// self-check distinguishing idle from stalled. They do not exercise
// Start() / Subscribe() because that requires a live bus goroutine;
// instead they drive the signals directly via the public methods.

func TestTraceProjector_Started_FalseBeforeStart(t *testing.T) {
	// Fresh projector must report Started()=false until Start() runs.
	// Self-check relies on this to detect wiring bugs.
	p := newTestTraceProjector(&mockTraceWriter{})
	if p.Started() {
		t.Error("expected Started()=false before Start()")
	}
}

func TestTraceProjector_LastEventAt_ZeroBeforeEvents(t *testing.T) {
	// No envelope has been processed, so LastEventAt must be the zero
	// time. The check uses this to detect "freshly started, never seen
	// traffic" vs "stalled".
	p := newTestTraceProjector(&mockTraceWriter{})
	if !p.LastEventAt().IsZero() {
		t.Errorf("expected zero time before any event, got %s", p.LastEventAt())
	}
	if p.HasEverProcessed() {
		t.Error("expected HasEverProcessed()=false before any event")
	}
}

func TestTraceProjector_HandleEnvelope_RecordsLastEvent(t *testing.T) {
	// handle() should set lastEventUnixNano even for envelopes without
	// a trace_id, because the bus subscription itself is the signal
	// that the projector is "alive".
	repo := &mockTraceWriter{}
	p := newTestTraceProjector(repo)
	// We don't call Start() (it spawns goroutines that race the test);
	// RecordEventForTest exercises the same lastEventUnixNano store
	// call that handle() makes on the hot path.
	p.RecordEventForTest()
	if p.LastEventAt().IsZero() {
		t.Error("expected LastEventAt to be non-zero after RecordEventForTest")
	}
	if !p.HasEverProcessed() {
		t.Error("expected HasEverProcessed()=true after RecordEventForTest")
	}
}

func TestTraceProjector_NilReceiverHealthMethods(t *testing.T) {
	// A nil *TraceProjector must not panic on the new health methods;
	// they all return the zero/false value, matching the contract used
	// by TraceCount() already.
	var p *trace.TraceProjector
	if p.Started() {
		t.Error("nil Started() should be false")
	}
	if !p.LastEventAt().IsZero() {
		t.Error("nil LastEventAt() should be zero")
	}
	if p.HasEverProcessed() {
		t.Error("nil HasEverProcessed() should be false")
	}
}
