package graph

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"

	trpcgraph "trpc.group/trpc-go/trpc-agent-go/graph"
)

func TestSkipNodeFunc_recordsSkippedNodes(t *testing.T) {
	fn := SkipNodeFunc("member-2")
	state := trpcgraph.State{}
	out, err := fn(context.Background(), state)
	if err != nil {
		t.Fatal(err)
	}
	skipped, ok := state[biz.SkippedNodesStateKey].([]string)
	if !ok || len(skipped) != 1 || skipped[0] != "member-2" {
		t.Fatalf("state=%v", state[biz.SkippedNodesStateKey])
	}
	m, ok := out.(map[string]any)
	if !ok || m[biz.SkippedNodeOutputKey] != "member-2" {
		t.Fatalf("out=%v", out)
	}
}

func TestBuildStateGraph_skipNode(t *testing.T) {
	cfg := GraphBuildConfig{
		EntryPoint:  "member-1",
		FinishPoint: "member-1",
		StateFields: []StateFieldDef{{Name: biz.SkippedNodesStateKey, Type: "[]string", Reducer: ReducerAppend}},
		Nodes:       []NodeDef{{ID: "member-1", Type: "function", FuncRef: biz.SkipNodeFuncRef}},
	}
	g, _, err := BuildStateGraphWithAgents(context.Background(), cfg, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if g == nil {
		t.Fatal("nil graph")
	}
}
