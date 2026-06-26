package event

import (
	"context"
	"sync"
	"testing"
	"time"

	"aranea-agents/internal/event/contract"
)

// captureMonitorBus captures MonitorEvents published to a contract.MonitorBus.
// Used by tests that verify flow_log publication after B-DEBT-1 removed the
// legacy Envelope fallback in FlowTracker.emit (flow_log now rides the typed
// MonitorEventBus, not the SessionBus Envelope bus).
type captureMonitorBus struct {
	mu  sync.Mutex
	evs []contract.MonitorEvent
}

func (b *captureMonitorBus) Publish(_ context.Context, ev contract.MonitorEvent) {
	b.mu.Lock()
	b.evs = append(b.evs, ev)
	b.mu.Unlock()
}

func (b *captureMonitorBus) Subscribe(_ contract.MonitorSubscribeOptions) (<-chan contract.MonitorEvent, func()) {
	return nil, func() {}
}

func (b *captureMonitorBus) DropCount() uint64 { return 0 }

func TestTraceEmitterPublishesFlowLog(t *testing.T) {
	tc := TraceContext{
		TraceID:   "tr_test",
		SessionID: "sess_1",
		RunID:     "run_1",
		Domain:    TraceDomainChat,
		AgentKey:  "a1",
	}
	em := NewTraceEmitter(tc, nil)
	// B-DEBT-1: flow_log now publishes via the typed MonitorEventBus, not the
	// legacy SessionBus Envelope fallback. Inject a captureMonitorBus so the
	// test verifies the new path.
	monBus := &captureMonitorBus{}
	em.FlowTracker.infra = &Infra{MonitorEventBus: monBus}
	em.LogStart("chat.llm.invoke", "正在调用语言模型")
	em.LogDone("chat.llm.invoke", "模型已返回")
	time.Sleep(50 * time.Millisecond)

	monBus.mu.Lock()
	defer monBus.mu.Unlock()
	if len(monBus.evs) < 2 {
		t.Fatalf("expected >=2 flow_log monitor events, got %d", len(monBus.evs))
	}
	for _, ev := range monBus.evs {
		if ev.Type != contract.MonitorEventTypeFlowLog {
			t.Fatalf("expected flow_log, got %s", ev.Type)
		}
		sev, _ := ev.Metadata["severity"].(string)
		if sev == "" {
			t.Fatal("missing severity in metadata")
		}
		title, _ := ev.Metadata["title"].(string)
		if title == "" {
			t.Fatal("missing title in metadata")
		}
	}
}

func TestTraceEmitterMetadataJSON(t *testing.T) {
	em := NewTraceEmitter(TraceContext{TraceID: "tr_x", RunID: "r1"}, nil)
	em.FinishRoot("ok")
	raw := em.MetadataJSON()
	if raw == "" || raw == "{}" {
		t.Fatal("expected metadata with spans")
	}
}
