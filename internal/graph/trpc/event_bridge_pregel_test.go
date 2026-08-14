package graph

import (
	"testing"

	"aranea-agents/pkg/loggateway"

	trpcgraph "trpc.group/trpc-go/trpc-agent-go/graph"
)

// N2/Y1: the Pregel interrupt event is the only reachable HITL carrier
// (checkpoint interrupt events require StreamModeCheckpoints which is not
// enabled). The bridge must surface interrupt metadata into the system.notice
// Meta so the team watch can mark the run waiting_human.
func TestConvertEvent_PregelInterrupt_SurfacesInterruptMeta(t *testing.T) {
	t.Parallel()
	b := NewEventBridge(nil, nil, "sess-1", "spirit-1", "graph-1", "exec-1", loggateway.NewNoop())
	e := trpcgraph.NewPregelInterruptEvent(
		trpcgraph.WithPregelEventInvocationID("inv-1"),
		trpcgraph.WithPregelEventStepNumber(3),
		trpcgraph.WithPregelEventNodeID("review-1"),
		trpcgraph.WithPregelEventInterruptKey("hitl"),
		trpcgraph.WithPregelEventInterruptValue(map[string]any{"prompt": "approve?"}),
		trpcgraph.WithPregelEventLineageID("lineage-1"),
	)
	ev := b.ConvertEvent(e)
	if ev == nil {
		t.Fatal("pregel interrupt must convert to an activity event")
	}
	if got := ev.Activity.Meta["interrupt_key"]; got != "hitl" {
		t.Fatalf("interrupt_key = %v, want hitl", got)
	}
	if got := ev.Activity.Meta["node_id"]; got != "review-1" {
		t.Fatalf("node_id = %v, want review-1", got)
	}
	if got := ev.Activity.Meta["lineage_id"]; got != "lineage-1" {
		t.Fatalf("lineage_id = %v, want lineage-1", got)
	}
	if _, ok := ev.Activity.Meta["interrupt_value"]; !ok {
		t.Fatal("interrupt_value must be preserved")
	}
}

// N3: graph-level fatal errors (max steps / panic / executeGraph failure)
// arrive as Pregel error events. The bridge must surface the error into
// system.notice Meta so the team watch can finalize the run as failed instead
// of waiting for the watch timeout.
func TestConvertEvent_PregelError_SurfacesErrorMeta(t *testing.T) {
	t.Parallel()
	b := NewEventBridge(nil, nil, "sess-1", "spirit-1", "graph-1", "exec-1", loggateway.NewNoop())
	e := trpcgraph.NewPregelErrorEvent(
		trpcgraph.WithPregelEventInvocationID("inv-1"),
		trpcgraph.WithPregelEventStepNumber(-1),
		trpcgraph.WithPregelEventError("graph execution exceeded max steps"),
	)
	ev := b.ConvertEvent(e)
	if ev == nil {
		t.Fatal("pregel error must convert to an activity event")
	}
	if got := ev.Activity.Meta["error"]; got != "graph execution exceeded max steps" {
		t.Fatalf("error = %v, want the pregel error message", got)
	}
}

// Plain step progress events must NOT carry error/interrupt keys.
func TestConvertEvent_PregelStepProgress_NoErrorNoInterrupt(t *testing.T) {
	t.Parallel()
	b := NewEventBridge(nil, nil, "sess-1", "spirit-1", "graph-1", "exec-1", loggateway.NewNoop())
	e := trpcgraph.NewPregelStepEvent(
		trpcgraph.WithPregelEventInvocationID("inv-1"),
		trpcgraph.WithPregelEventStepNumber(1),
	)
	ev := b.ConvertEvent(e)
	if ev == nil {
		t.Fatal("pregel step progress must convert to an activity event")
	}
	if _, ok := ev.Activity.Meta["error"]; ok {
		t.Fatal("progress step must not carry an error key")
	}
	if _, ok := ev.Activity.Meta["interrupt_key"]; ok {
		t.Fatal("progress step must not carry an interrupt key")
	}
}
