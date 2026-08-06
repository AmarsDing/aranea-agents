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
		Nodes:       []biz.NodeDef{{ID: "member-1", Type: "function", FuncRef: biz.SkipNodeFuncRef}},
	}
	g, _, err := BuildStateGraphWithAgents(context.Background(), cfg, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if g == nil {
		t.Fatal("nil graph")
	}
}

// TestRegistryResolveNodeDef_skipNodeFuncRef 覆盖 20:45 会话编排失败的根因：
// biz.SkipNodeFuncRef（"orchestration.skip"）是内置 sentinel，由
// biz.ApplySkipNodeSemantics 注入到 verification 节点的 FuncRef。wiring 路径
// 有特判，但 Registry.ResolveNodeDef 此前直接查表，内置 sentinel 从未注册，
// 导致带 registry 的构建（build_orchestration_graph）必报 NOT_FOUND。
// Registry 必须与 node_wiring.go 对齐：特判返回 SkipNodeFunc(n.ID)。
func TestRegistryResolveNodeDef_skipNodeFuncRef(t *testing.T) {
	reg := NewRegistry()
	nd, err := reg.ResolveNodeDef(biz.NodeDef{
		ID:      "verify_output_format_0",
		Type:    biz.NodeTypeFunction,
		FuncRef: biz.SkipNodeFuncRef,
	})
	if err != nil {
		t.Fatalf("ResolveNodeDef should special-case built-in skip sentinel, got: %v", err)
	}
	if nd.Func == nil {
		t.Fatal("resolved Func must be SkipNodeFunc, got nil")
	}
	// 行为与 wiring 路径一致：执行后记录 skipped node。
	state := trpcgraph.State{}
	out, err := nd.Func(context.Background(), state)
	if err != nil {
		t.Fatal(err)
	}
	skipped, ok := state[biz.SkippedNodesStateKey].([]string)
	if !ok || len(skipped) != 1 || skipped[0] != "verify_output_format_0" {
		t.Fatalf("state=%v", state[biz.SkippedNodesStateKey])
	}
	m, ok := out.(map[string]any)
	if !ok || m[biz.SkippedNodeOutputKey] != "verify_output_format_0" {
		t.Fatalf("out=%v", out)
	}
}

// TestBuildStateGraphWithRegistry_skipNode 端到端覆盖：带 registry 的构建
// 路径（ResolveBuildConfig）遇到 skip 节点必须成功——与不带 registry 的
// BuildStateGraphWithAgents 行为一致。
func TestBuildStateGraphWithRegistry_skipNode(t *testing.T) {
	cfg := GraphBuildConfig{
		EntryPoint:  "verify_output_format_0",
		FinishPoint: "verify_output_format_0",
		StateFields: []StateFieldDef{{Name: biz.SkippedNodesStateKey, Type: "[]string", Reducer: ReducerAppend}},
		Nodes:       []biz.NodeDef{{ID: "verify_output_format_0", Type: biz.NodeTypeFunction, FuncRef: biz.SkipNodeFuncRef}},
	}
	g, _, _, err := BuildStateGraphWithRegistryAndNodeAgents(context.Background(), cfg, NewRegistry(), nil, nil)
	if err != nil {
		t.Fatalf("registry path must resolve built-in skip node, got: %v", err)
	}
	if g == nil {
		t.Fatal("nil graph")
	}
}
