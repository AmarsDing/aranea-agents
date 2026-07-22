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
	// B-DEBT-1: flow_log publishes via the typed MonitorEventBus. Inject a
	// captureMonitorBus at construction so the test verifies the new path.
	monBus := &captureMonitorBus{}
	em := NewTraceEmitter(&Infra{MonitorEventBus: monBus}, tc, nil)
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
	em := NewTraceEmitter(nil, TraceContext{TraceID: "tr_x", RunID: "r1"}, nil)
	em.FinishRoot("ok")
	raw := em.MetadataJSON()
	if raw == "" || raw == "{}" {
		t.Fatal("expected metadata with spans")
	}
}

// TestFlowLogEntry_carriesSpanIDFromOtelRefs verifies Phase 1 of Problem 4:
// after SetOtelRefs, each emitted FlowLog carries span_id == rootSpanID,
// enabling cross-reference between FlowLog and OTel trace (Jaeger).
func TestFlowLogEntry_carriesSpanIDFromOtelRefs(t *testing.T) {
	tc := TraceContext{
		TraceID:   "tr_test",
		SessionID: "sess_1",
		RunID:     "run_1",
		Domain:    TraceDomainChat,
	}
	monBus := &captureMonitorBus{}
	em := NewTraceEmitter(&Infra{MonitorEventBus: monBus}, tc, nil)
	em.SetOtelRefs("tr_test", "otel_root_span_123")
	em.LogDone("chat.llm.invoke", "model returned")
	time.Sleep(50 * time.Millisecond)

	monBus.mu.Lock()
	defer monBus.mu.Unlock()
	if len(monBus.evs) == 0 {
		t.Fatal("expected flow_log monitor events")
	}
	for _, ev := range monBus.evs {
		spanID, _ := ev.Metadata["span_id"].(string)
		if spanID != "otel_root_span_123" {
			t.Fatalf("expected span_id=otel_root_span_123, got %q (metadata: %+v)", spanID, ev.Metadata)
		}
	}
}

// TestFlowLogEntry_emptySpanIDWhenOtelRefsNotSet verifies graceful degradation:
// without SetOtelRefs, FlowLog entries still emit with empty span_id (no crash).
func TestFlowLogEntry_emptySpanIDWhenOtelRefsNotSet(t *testing.T) {
	tc := TraceContext{
		TraceID:   "tr_test",
		SessionID: "sess_1",
		RunID:     "run_1",
		Domain:    TraceDomainChat,
	}
	monBus := &captureMonitorBus{}
	em := NewTraceEmitter(&Infra{MonitorEventBus: monBus}, tc, nil)
	em.LogDone("chat.llm.invoke", "model returned")
	time.Sleep(50 * time.Millisecond)

	monBus.mu.Lock()
	defer monBus.mu.Unlock()
	if len(monBus.evs) == 0 {
		t.Fatal("expected flow_log monitor events")
	}
	for _, ev := range monBus.evs {
		if spanID, _ := ev.Metadata["span_id"].(string); spanID != "" {
			t.Fatalf("expected empty span_id when OtelRefs not set, got %q", spanID)
		}
	}
}

// TestNewTraceEmitterForRunPublishesViaInfraFromBus verifies the production
// wiring path end-to-end: call sites wrap the shared contract.MonitorBus with
// NewInfraFromBus and pass it via TraceEmitterOpts.Infra, so flow_log events
// reach the same bus the WS server subscribes to. Regression test for the
// missing-infra bug where flow_log events were silently dropped (ft.infra == nil).
func TestNewTraceEmitterForRunPublishesViaInfraFromBus(t *testing.T) {
	monBus := &captureMonitorBus{}
	em := NewTraceEmitterForRun(TraceEmitterOpts{
		Ctx:       context.Background(),
		SessionID: "sess_e2e",
		RunID:     "run_e2e",
		AgentKey:  "agent_x",
		Domain:    TraceDomainChat,
		Infra:     NewInfraFromBus(monBus),
	})
	em.LogStart("chat.llm.invoke", "正在调用语言模型")
	em.LogDone("chat.llm.invoke", "模型已返回")

	monBus.mu.Lock()
	defer monBus.mu.Unlock()
	if len(monBus.evs) < 2 {
		t.Fatalf("expected >=2 flow_log events via NewInfraFromBus path, got %d", len(monBus.evs))
	}
	for _, ev := range monBus.evs {
		if ev.Type != contract.MonitorEventTypeFlowLog {
			t.Fatalf("expected flow_log, got %s", ev.Type)
		}
		if ev.SessionID != "sess_e2e" {
			t.Fatalf("expected session_id sess_e2e, got %q", ev.SessionID)
		}
	}
}
