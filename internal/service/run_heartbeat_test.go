package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// heartbeatCaptureBus is a thread-safe ActivityEventBus that captures published events.
// Unlike gateCaptureBus, it uses a mutex because heartbeats are published
// from a goroutine.
type heartbeatCaptureBus struct {
	mu        sync.Mutex
	published []biz.ActivityEvent
}

func (b *heartbeatCaptureBus) Publish(_ context.Context, ev biz.ActivityEvent) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.published = append(b.published, ev)
}

func (b *heartbeatCaptureBus) Subscribe(_ biz.ActivityEventSubscribeOptions) (<-chan biz.ActivityEvent, func()) {
	return nil, func() {}
}

func (b *heartbeatCaptureBus) DropCount() uint64 { return 0 }

func (b *heartbeatCaptureBus) snapshot() []biz.ActivityEvent {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]biz.ActivityEvent, len(b.published))
	copy(out, b.published)
	return out
}

func (b *heartbeatCaptureBus) count() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.published)
}

// TestRunHeartbeatEmitter_Start_PublishesPeriodically verifies that the
// emitter publishes heartbeat events at each tick with correct metadata.
func TestRunHeartbeatEmitter_Start_PublishesPeriodically(t *testing.T) {
	bus := &heartbeatCaptureBus{}
	e := NewRunHeartbeatEmitter(20*time.Millisecond, bus, loggateway.NewNoop())

	progress := func() RunProgress {
		return RunProgress{Percent: 0.5, CurrentStep: "step1", TotalSteps: 10, ETA: "5s"}
	}

	stop := e.Start(context.Background(), "run-1", "sess-1", progress)
	defer stop()

	time.Sleep(150 * time.Millisecond) // ~7 ticks at 20ms

	published := bus.snapshot()
	if len(published) < 2 {
		t.Fatalf("expected at least 2 heartbeats, got %d", len(published))
	}

	ev := published[0]
	if ev.Activity.Kind != biz.ActivityKindNotice {
		t.Errorf("kind = %s, want %s", ev.Activity.Kind, biz.ActivityKindNotice)
	}
	if ev.Activity.SessionID != "sess-1" {
		t.Errorf("session_id = %s, want sess-1", ev.Activity.SessionID)
	}
	if ev.Activity.Meta["run_id"] != "run-1" {
		t.Errorf("run_id = %v, want run-1", ev.Activity.Meta["run_id"])
	}
	if ev.Activity.Meta["progress_percent"] != 0.5 {
		t.Errorf("progress_percent = %v, want 0.5", ev.Activity.Meta["progress_percent"])
	}
	if ev.Activity.Meta["current_step"] != "step1" {
		t.Errorf("current_step = %v, want step1", ev.Activity.Meta["current_step"])
	}
	if ev.Activity.Meta["total_steps"] != 10 {
		t.Errorf("total_steps = %v, want 10", ev.Activity.Meta["total_steps"])
	}
	if ev.Activity.Meta["eta"] != "5s" {
		t.Errorf("eta = %v, want 5s", ev.Activity.Meta["eta"])
	}
	if ev.Domain != biz.ActivityDomainSystem {
		t.Errorf("domain = %s, want %s", ev.Domain, biz.ActivityDomainSystem)
	}
}

// TestRunHeartbeatEmitter_Start_StopCancels verifies that calling the stop
// function stops the ticker and no further events are published.
func TestRunHeartbeatEmitter_Start_StopCancels(t *testing.T) {
	bus := &heartbeatCaptureBus{}
	e := NewRunHeartbeatEmitter(20*time.Millisecond, bus, loggateway.NewNoop())

	stop := e.Start(context.Background(), "run-1", "sess-1", nil)

	time.Sleep(50 * time.Millisecond) // ~2 ticks
	stop()

	countBefore := bus.count()
	time.Sleep(100 * time.Millisecond) // wait to see if more events come

	countAfter := bus.count()
	if countAfter > countBefore {
		t.Errorf("events published after stop: before=%d after=%d", countBefore, countAfter)
	}
}

// TestRunHeartbeatEmitter_Start_NilProgress verifies that a nil progress
// function does not panic and publishes a heartbeat with only run_id.
func TestRunHeartbeatEmitter_Start_NilProgress(t *testing.T) {
	bus := &heartbeatCaptureBus{}
	e := NewRunHeartbeatEmitter(20*time.Millisecond, bus, loggateway.NewNoop())

	stop := e.Start(context.Background(), "run-1", "sess-1", nil)
	defer stop()

	time.Sleep(50 * time.Millisecond)

	published := bus.snapshot()
	if len(published) == 0 {
		t.Fatal("expected at least 1 heartbeat")
	}
	ev := published[0]
	if ev.Activity.Meta["run_id"] != "run-1" {
		t.Errorf("run_id = %v, want run-1", ev.Activity.Meta["run_id"])
	}
	if _, ok := ev.Activity.Meta["progress_percent"]; ok {
		t.Error("progress_percent should be absent for nil progress")
	}
	if _, ok := ev.Activity.Meta["current_step"]; ok {
		t.Error("current_step should be absent for nil progress")
	}
}

// TestRunHeartbeatEmitter_Start_DefaultInterval verifies that interval <= 0
// falls back to the default 10s interval.
func TestRunHeartbeatEmitter_Start_DefaultInterval(t *testing.T) {
	e := NewRunHeartbeatEmitter(0, &heartbeatCaptureBus{}, nil)
	if e.interval != defaultHeartbeatInterval {
		t.Errorf("interval = %v, want %v", e.interval, defaultHeartbeatInterval)
	}

	e2 := NewRunHeartbeatEmitter(-1, &heartbeatCaptureBus{}, nil)
	if e2.interval != defaultHeartbeatInterval {
		t.Errorf("interval = %v, want %v", e2.interval, defaultHeartbeatInterval)
	}
}

// TestRunHeartbeatEmitter_Start_ContextCancel verifies that cancelling the
// context causes the goroutine to exit and no further events are published.
func TestRunHeartbeatEmitter_Start_ContextCancel(t *testing.T) {
	bus := &heartbeatCaptureBus{}
	e := NewRunHeartbeatEmitter(20*time.Millisecond, bus, loggateway.NewNoop())

	ctx, cancel := context.WithCancel(context.Background())
	stop := e.Start(ctx, "run-1", "sess-1", nil)
	defer stop() // safe to call even after cancel

	time.Sleep(50 * time.Millisecond)
	cancel()

	countBefore := bus.count()
	time.Sleep(100 * time.Millisecond)

	countAfter := bus.count()
	if countAfter > countBefore {
		t.Errorf("events published after ctx cancel: before=%d after=%d", countBefore, countAfter)
	}
}
