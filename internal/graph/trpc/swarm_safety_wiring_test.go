package graph

import (
	"strings"
	"testing"

	"aranea-agents/internal/biz"

	trpcgraph "trpc.group/trpc-go/trpc-agent-go/graph"
)

func TestEnforceSwarmSafety_MaxHandoffs(t *testing.T) {
	state := trpcgraph.State{}
	delta, err := enforceSwarmSafety(state, "m2", "entry", 1, 0, 0)
	if err != nil {
		t.Fatalf("first handoff: %v", err)
	}
	mergeSwarmDelta(state, delta)

	_, err = enforceSwarmSafety(state, "m2", "entry", 1, 0, 0)
	if err == nil {
		t.Fatal("expected max handoffs error")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "max handoffs") {
		t.Fatalf("unexpected err: %v", err)
	}
}

func TestEnforceSwarmSafety_Repetitive(t *testing.T) {
	state := trpcgraph.State{}
	delta, err := enforceSwarmSafety(state, "a", "entry", 0, 2, 2)
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	mergeSwarmDelta(state, delta)

	_, err = enforceSwarmSafety(state, "a", "entry", 0, 2, 2)
	if err == nil {
		t.Fatal("expected repetitive handoff error")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "repetitive handoff") {
		t.Fatalf("unexpected err: %v", err)
	}
}

func TestEnforceSwarmSafety_SkipsEntry(t *testing.T) {
	delta, err := enforceSwarmSafety(trpcgraph.State{}, "entry", "entry", 1, 2, 2)
	if err != nil {
		t.Fatalf("entry should skip: %v", err)
	}
	if delta != nil {
		t.Fatalf("entry should not update state, got %v", delta)
	}
}

func TestSwarmSafetyOptions_WiredForAgent(t *testing.T) {
	cfg := biz.GraphBuildConfig{
		SwarmSafety: &biz.SwarmSafetySpec{MaxHandoffs: 3},
	}
	n := NodeDef{NodeDef: biz.NodeDef{ID: "m1", Type: biz.NodeTypeAgent}}
	if opts := swarmSafetyOptions(n, cfg); len(opts) != 1 {
		t.Fatalf("agent should get swarm options, got %d", len(opts))
	}
	n.Type = biz.NodeTypeRouter
	if opts := swarmSafetyOptions(n, cfg); len(opts) != 0 {
		t.Fatalf("router should skip, got %d", len(opts))
	}
}

func TestMaxNodeTimeout(t *testing.T) {
	if got := MaxNodeTimeout(nil); got != 0 {
		t.Fatalf("nil => 0, got %v", got)
	}
	nodes := []biz.NodeDef{
		{ID: "a", TimeoutSeconds: 10},
		{ID: "b", TimeoutSeconds: 45},
		{ID: "c", TimeoutSeconds: 20},
	}
	if got := MaxNodeTimeout(nodes); got.Seconds() != 45 {
		t.Fatalf("want 45s, got %v", got)
	}
}

func mergeSwarmDelta(dst trpcgraph.State, delta trpcgraph.State) {
	for k, v := range delta {
		dst[k] = v
	}
}
