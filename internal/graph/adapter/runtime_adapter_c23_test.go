package adapter

import (
	"errors"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/graph"

	trpcgraph "trpc.group/trpc-go/trpc-agent-go/graph"
)

func TestApplyReplanControl_RetryReturnsControlCommandNotFakeString(t *testing.T) {
	t.Parallel()
	state := trpcgraph.State{}
	cause := errors.New("transient timeout")
	action := &graph.ReplanAction{Type: graph.ReplanRetry}

	out, err := applyReplanControl(state, "node-a", cause, action)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	cmd, ok := graph.AsControlCommand(out)
	if !ok {
		t.Fatalf("expected ControlCommand, got %T (%v)", out, out)
	}
	if cmd.Action != graph.ReplanRetry {
		t.Fatalf("action=%q want %q", cmd.Action, graph.ReplanRetry)
	}
	if cmd.NodeID != "node-a" {
		t.Fatalf("node_id=%q", cmd.NodeID)
	}
	if !cmd.AttemptAllowed {
		t.Fatal("AttemptAllowed should be true when replanner returned an action")
	}
	if _, isStr := out.(string); isStr {
		t.Fatal("must not return fake success string")
	}
	stored, ok := state[graph.StateKeyControlCommand]
	if !ok {
		t.Fatal("expected ControlCommand in state")
	}
	if _, ok := graph.AsControlCommand(stored); !ok {
		t.Fatalf("state value not ControlCommand: %T", stored)
	}
}

func TestApplyReplanControl_InsertFallbackStructuredSignal(t *testing.T) {
	t.Parallel()
	state := trpcgraph.State{}
	action := &graph.ReplanAction{
		Type: graph.ReplanInsertFallback,
		NewNodes: []biz.NodeDef{
			{ID: "member-1_fallback", Type: biz.NodeTypeAgent, AgentName: "backup-agent"},
		},
	}

	out, err := applyReplanControl(state, "member-1", errors.New("agent incapable"), action)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	cmd, ok := graph.AsControlCommand(out)
	if !ok {
		t.Fatalf("expected ControlCommand, got %T", out)
	}
	if cmd.Action != graph.ReplanInsertFallback {
		t.Fatalf("action=%q", cmd.Action)
	}
	if cmd.FallbackAgent != "backup-agent" {
		t.Fatalf("FallbackAgent=%q want backup-agent", cmd.FallbackAgent)
	}
}

func TestApplyReplanControl_ReroutePropagatesNil(t *testing.T) {
	t.Parallel()
	out, err := applyReplanControl(nil, "n", errors.New("blocked"), &graph.ReplanAction{Type: graph.ReplanReroute})
	if err != nil || out != nil {
		t.Fatalf("want (nil,nil), got (%v, %v)", out, err)
	}
}
