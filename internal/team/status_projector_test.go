package team

import (
	"context"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
)

func TestStartOrchestrationStatusProjector_PublishesStatus(t *testing.T) {
	bus := event.NewBus()
	reg := biz.NewOrchestrationRegistry([]biz.OrchestrationNodeRegistryEntry{
		{NodeID: "member-1", AgentID: "a1", AgentKey: "worker-a", AgentName: "A", Role: "worker"},
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	outCh, unsub := bus.Subscribe(event.SubscribeOptions{
		SessionID:  "sess-1",
		EventTypes: []event.EnvelopeType{event.EnvelopeTypeOrchestrationAgentStatus},
		BufferSize: 8,
	})
	defer unsub()

	stop := StartOrchestrationStatusProjector(ctx, bus, OrchestrationProjectorConfig{
		RunID: "run-1", TeamID: "team-1", SessionID: "sess-1", Registry: reg,
	})
	defer stop()

	env := event.NewEnvelope(event.EnvelopeTypeMemberMessageStart, "worker-a", "sess-1")
	env.ToolCall = &event.EnvelopeToolCall{AgentKey: "worker-a", AgentID: "a1"}
	bus.Publish(ctx, env)

	select {
	case got := <-outCh:
		if got.Metadata["status"] != string(biz.AgentNodeStatusThinking) {
			t.Fatalf("status: %v", got.Metadata["status"])
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for orchestration_agent_status")
	}
}
