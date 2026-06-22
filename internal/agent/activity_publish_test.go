package agent

import (
	"context"
	"testing"

	"aranea-agents/internal/event"
	"aranea-agents/internal/event/contract"
)

type captureBus struct {
	published []event.Envelope
}

func (b *captureBus) Publish(_ context.Context, env event.Envelope) {
	b.published = append(b.published, env)
}

func (b *captureBus) Subscribe(_ contract.SubscribeOptions) (<-chan event.Envelope, func()) {
	return nil, func() {}
}

func (b *captureBus) DropCount() uint64 { return 0 }

func TestPublishStuckToolResultEnvelopes_emitsFailedToolResult(t *testing.T) {
	bus := &captureBus{}
	infra := &event.Infra{SessionBus: bus}
	meta := ProjectMeta{SessionID: "sess-1", RequestID: "req-1"}
	pending := map[string]event.EnvelopeToolCall{
		"tc-1": {ID: "tc-1", Name: "read_file", Status: "running"},
	}
	PublishStuckToolResultEnvelopes(context.Background(), meta, infra, pending)
	if len(bus.published) != 1 {
		t.Fatalf("published=%d want 1", len(bus.published))
	}
	env := bus.published[0]
	if env.Type != event.EnvelopeTypeToolResult {
		t.Fatalf("type=%q want tool_result", env.Type)
	}
	if env.ToolCall == nil || env.ToolCall.Status != "failed" {
		t.Fatalf("tool status=%v want failed", env.ToolCall)
	}
}
