package adapter

import (
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"

	trpcevent "trpc.group/trpc-go/trpc-agent-go/event"
	trpcgraph "trpc.group/trpc-go/trpc-agent-go/graph"
	"trpc.group/trpc-go/trpc-agent-go/model"
)

// N1: framework fatal errors (panic / max steps / executeGraph failures) arrive
// as Pregel error events (Object=graph.pregel.step). convertTrpcEvent must
// surface them as execution-level failures — dropping them lets the execution
// converge to Completed (false success).
func TestConvertTrpcEvent_PregelError_MapsToExecutionError(t *testing.T) {
	t.Parallel()
	e := trpcgraph.NewPregelErrorEvent(
		trpcgraph.WithPregelEventInvocationID("inv-1"),
		trpcgraph.WithPregelEventStepNumber(-1),
		trpcgraph.WithPregelEventError("graph execution exceeded max steps"),
	)
	got := convertTrpcEvent(e, nil, loggateway.NewNoop())
	if got.Type != biz.DomainEventGraphExecutionError {
		t.Fatalf("Type = %q, want %q", got.Type, biz.DomainEventGraphExecutionError)
	}
	if got.Error == "" {
		t.Fatal("Error must carry the pregel error message")
	}
}

// N2: HITL interrupts arrive as Pregel interrupt events — the checkpoint
// interrupt event requires StreamModeCheckpoints which the biz adapter does not
// enable, so Pregel interrupt is the only reachable interrupt carrier.
func TestConvertTrpcEvent_PregelInterrupt_MapsToInterrupt(t *testing.T) {
	t.Parallel()
	e := trpcgraph.NewPregelInterruptEvent(
		trpcgraph.WithPregelEventInvocationID("inv-1"),
		trpcgraph.WithPregelEventStepNumber(3),
		trpcgraph.WithPregelEventNodeID("review-1"),
		trpcgraph.WithPregelEventInterruptKey("hitl"),
		trpcgraph.WithPregelEventInterruptValue(map[string]any{"prompt": "approve?"}),
		trpcgraph.WithPregelEventLineageID("lineage-1"),
	)
	got := convertTrpcEvent(e, nil, loggateway.NewNoop())
	if got.Type != biz.DomainEventGraphInterrupt {
		t.Fatalf("Type = %q, want %q", got.Type, biz.DomainEventGraphInterrupt)
	}
	if got.NodeID != "review-1" {
		t.Fatalf("NodeID = %q, want review-1", got.NodeID)
	}
}

// Plain pregel step progress events must NOT produce a runtime event type.
func TestConvertTrpcEvent_PregelStepProgress_NoType(t *testing.T) {
	t.Parallel()
	e := trpcgraph.NewPregelStepEvent(
		trpcgraph.WithPregelEventInvocationID("inv-1"),
		trpcgraph.WithPregelEventStepNumber(1),
	)
	got := convertTrpcEvent(e, nil, loggateway.NewNoop())
	if got.Type != "" {
		t.Fatalf("progress step must not map to a domain event, got %q", got.Type)
	}
}

// N1 follow-up: execution completion must be explicit (done-driven) — the
// framework's terminal graph.execution event maps to DomainEventGraphDone.
func TestConvertTrpcEvent_GraphExecutionDone_MapsToDone(t *testing.T) {
	t.Parallel()
	e := &trpcevent.Event{Response: &model.Response{Object: trpcgraph.ObjectTypeGraphExecution, Done: true}}
	got := convertTrpcEvent(e, nil, loggateway.NewNoop())
	if got.Type != biz.DomainEventGraphDone {
		t.Fatalf("Type = %q, want %q", got.Type, biz.DomainEventGraphDone)
	}
}

// Non-terminal graph.execution events (running updates) must stay untyped.
func TestConvertTrpcEvent_GraphExecutionNotDone_NoType(t *testing.T) {
	t.Parallel()
	e := &trpcevent.Event{Response: &model.Response{Object: trpcgraph.ObjectTypeGraphExecution, Done: false}}
	got := convertTrpcEvent(e, nil, loggateway.NewNoop())
	if got.Type != "" {
		t.Fatalf("non-done execution event must not map, got %q", got.Type)
	}
}
