package graph

import (
	"context"
	"errors"
	"testing"

	"aranea-agents/internal/biz"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcgraph "trpc.group/trpc-go/trpc-agent-go/graph"
)

func TestBuildStateGraph_parallelFailContinue(t *testing.T) {
	pass := func(_ context.Context, _ trpcgraph.State) (any, error) {
		return map[string]any{"ok": true}, nil
	}
	fail := func(_ context.Context, _ trpcgraph.State) (any, error) {
		return nil, errors.New("branch failed")
	}
	cfg := GraphBuildConfig{
		EntryPoint:  "member-1",
		FinishPoint: "member-3",
		StateFields: []StateFieldDef{{Name: biz.SkippedNodesStateKey, Type: "[]string", Reducer: ReducerAppend}},
		Nodes: []NodeDef{
			{ID: "member-1", Type: "function", Func: pass},
			{ID: "member-2", Type: "function", Func: fail, FailureAction: biz.FailureOnFailureSkip},
			{ID: "member-3", Type: "function", Func: pass},
		},
		Edges: []EdgeDef{
			{From: "member-1", To: "member-2"},
			{From: "member-1", To: "member-3"},
			{From: "member-2", To: "member-3"},
		},
	}
	g, _, _, err := BuildStateGraphWithAgents(context.Background(), cfg, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	exec, err := trpcgraph.NewExecutor(g)
	if err != nil {
		t.Fatal(err)
	}
	ch, err := exec.Execute(context.Background(), trpcgraph.State{}, &trpcagent.Invocation{InvocationID: "inv-parallel-continue"})
	if err != nil {
		t.Fatal(err)
	}
	var done bool
	var nodeErrors int
	for ev := range ch {
		if ev.Done {
			done = true
		}
		if ev.Object == trpcgraph.ObjectTypeGraphNodeError {
			nodeErrors++
		}
	}
	if !done {
		t.Fatal("expected graph to complete when parallel branch skips on failure")
	}
	if nodeErrors == 0 {
		t.Fatal("expected at least one node error event before skip recovery")
	}
}
