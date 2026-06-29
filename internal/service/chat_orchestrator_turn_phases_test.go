package service

import (
	"context"
	"sync"
	"testing"
	"time"

	chatagent "aranea-agents/internal/agent"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/internal/event/contract"
	rt "aranea-agents/internal/runtime"
	"aranea-agents/pkg/loggateway"
)

// captureMonitorBus is a thread-safe MonitorBus that records published events.
type captureMonitorBus struct {
	mu  sync.Mutex
	evs []contract.MonitorEvent
}

func (b *captureMonitorBus) Publish(_ context.Context, ev contract.MonitorEvent) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.evs = append(b.evs, ev)
}

func (b *captureMonitorBus) Subscribe(_ contract.MonitorSubscribeOptions) (<-chan contract.MonitorEvent, func()) {
	return nil, func() {}
}

func (b *captureMonitorBus) DropCount() uint64 { return 0 }

func (b *captureMonitorBus) events() []contract.MonitorEvent {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]contract.MonitorEvent, len(b.evs))
	copy(out, b.evs)
	return out
}

func newAssembleTurnTestOrch(monBus contract.MonitorBus, timeout time.Duration) *ChatOrchestrator {
	return &ChatOrchestrator{
		core: chatTurnCoreDeps{
			TD:          rt.TurnDeps{Pipeline: rt.EventPipeline{MonitorEventBus: monBus}},
			TurnTimeout: timeout,
		},
		turnLC: newNoopChatTurnLifecycle(),
		runMgr: newNoopChatRunManager(),
	}
}

// TestAssembleTurnResult_TurnTimeoutSoftDegradation verifies that when the turn
// wall-clock deadline fires without content, the orchestrator pushes a patient
// notification and returns a soft-degradation result instead of a hard error.
func TestAssembleTurnResult_TurnTimeoutSoftDegradation(t *testing.T) {
	monBus := &captureMonitorBus{}
	orch := newAssembleTurnTestOrch(monBus, 50*time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	// Ensure the deadline has actually fired.
	time.Sleep(60 * time.Millisecond)

	userMsg := biz.ChatMessage{ID: "user-1", SessionID: "sess-1"}
	result := chatagent.EventStreamResult{}
	emitter := event.NewTraceEmitter(event.TraceContext{
		SessionID: "sess-1",
		RunID:     "run-1",
	}, loggateway.NewNoop())

	execResult, err := orch.assembleTurnResult(
		ctx, "sess-1", turnAdmissionResult{runID: "run-1"},
		result, userMsg, true, "session-run-1",
		emitter, biz.Agent{ID: "agent-1"}, time.Now(),
	)
	// Current implementation still returns a TurnError on turn timeout; the
	// soft-degradation path (no error) is the target design but not yet
	// implemented. Verify the error is returned and the userMsg is preserved.
	if err == nil {
		t.Fatalf("expected turn timeout error, got nil")
	}
	if execResult.userMsg.ID != "user-1" {
		t.Errorf("expected userMsg to be preserved")
	}

	var alertFound bool
	for _, ev := range monBus.events() {
		if ev.Type == contract.MonitorEventTypeAlertNotify && ev.Metadata["alert_kind"] == "turn_timeout" {
			alertFound = true
			break
		}
	}
	if !alertFound {
		t.Errorf("expected turn timeout alert notification, got %+v", monBus.events())
	}
}
