package agent

import (
	"context"
	"testing"

	"aranea-agents/internal/agent/v2"
	"aranea-agents/pkg/loggateway"

	trpcevent "trpc.group/trpc-go/trpc-agent-go/event"
)

// TestStreamConsumer_V2DualPath is a placeholder integration test for the v2
// dual-path dispatch in turnStreamConsumer.
//
// It verifies that when a v2 projector is wired via StreamConsumeOptions,
// trpc events are dispatched to both v1 and v2 projectors, and the v2
// projector emits the expected v2 events (task.created, turn.started,
// step.created, step.streaming, step.completed, turn.completed, task.completed).
//
// SKIPPED: The full integration requires a realistic trpc event stream
// (chat.completion.chunk events with choices/deltas). The v2 projector's
// event translation is unit-tested in internal/agent/v2/projector_test.go;
// this test will be enabled once the v2 event stream harness is available.
func TestStreamConsumer_V2DualPath(t *testing.T) {
	t.Skip("v2 dual-path integration test: requires trpc event stream harness (see v2/projector_test.go for unit coverage)")

	ctx := context.Background()
	events := make(chan *trpcevent.Event)
	go func() {
		defer close(events)
		// TODO: emit a realistic chat.completion.chunk sequence here.
	}()

	v2Proj := v2.NewActivityProjector(nil, v2.NewDefaultSeqAssigner(), loggateway.NewNoop())
	opts := &StreamConsumeOptions{
		V2Projector: v2Proj,
	}
	meta := ProjectMeta{
		SessionID:        "sess-1",
		SpiritSessionID: "sess-1",
		RequestID:        "task-1",
		InvocationID:     "turn-1",
		AgentID:          "agent-1",
		TaskContent:       "hello",
	}

	_ = ConsumeEventStream(ctx, events, meta, opts, loggateway.NewNoop())
	// TODO: assert v2 events were emitted via a capturing sequencer.
}
