package event

import (
	"context"
	"sync"
	"testing"
	"time"
)

type captureBus struct {
	mu   sync.Mutex
	envs []Envelope
}

func (b *captureBus) Publish(_ context.Context, env Envelope) {
	b.mu.Lock()
	b.envs = append(b.envs, env)
	b.mu.Unlock()
}

func (b *captureBus) Subscribe(_ SubscribeOptions) (<-chan Envelope, func()) {
	return nil, func() {}
}

func (b *captureBus) DropCount() uint64 { return 0 }

func TestTraceEmitterPublishesFlowLog(t *testing.T) {
	bus := &captureBus{}
	tc := TraceContext{
		TraceID:   "tr_test",
		SessionID: "sess_1",
		RunID:     "run_1",
		Domain:    TraceDomainChat,
		AgentKey:  "a1",
	}
	em := NewTraceEmitter(bus, nil, tc)
	em.LogStart("chat.llm.invoke", "正在调用语言模型")
	em.LogDone("chat.llm.invoke", "模型已返回")
	time.Sleep(50 * time.Millisecond)

	bus.mu.Lock()
	defer bus.mu.Unlock()
	if len(bus.envs) < 2 {
		t.Fatalf("expected >=2 flow_log envelopes, got %d", len(bus.envs))
	}
	for _, env := range bus.envs {
		if env.Type != EnvelopeTypeFlowLog {
			t.Fatalf("expected flow_log, got %s", env.Type)
		}
		sev, _ := env.Metadata["severity"].(string)
		if sev == "" {
			t.Fatal("missing severity in metadata")
		}
		title, _ := env.Metadata["title"].(string)
		if title == "" {
			t.Fatal("missing title in metadata")
		}
	}
}

func TestTraceEmitterMetadataJSON(t *testing.T) {
	em := NewTraceEmitter(nil, nil, TraceContext{TraceID: "tr_x", RunID: "r1"})
	em.FinishRoot("ok")
	raw := em.MetadataJSON()
	if raw == "" || raw == "{}" {
		t.Fatal("expected metadata with spans")
	}
}
