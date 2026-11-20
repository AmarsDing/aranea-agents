package graph

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"

	trpcgraph "trpc.group/trpc-go/trpc-agent-go/graph"
)

// 内置模板建出来的图：校验不报「函数节点没绑定函数」，并且真能建图（此前校验通过、一执行就失败）。
func TestBuiltinTemplates_ValidateAndBuild(t *testing.T) {
	for _, tmpl := range ListBuiltinTemplates() {
		t.Run(tmpl.ID, func(t *testing.T) {
			cfg := TemplateToBuildConfig(tmpl)
			res := ValidateGraph(context.Background(), &cfg, nil, NewRegistry())
			for _, e := range res.Errors {
				if e.Code == ValidationErrFuncRefNotFound {
					t.Fatalf("template %s: %s", tmpl.ID, e.Message)
				}
			}
			hasAgent := false
			for _, n := range cfg.Nodes {
				if n.Type == "agent" || n.Type == "llm" {
					hasAgent = true
				}
			}
			if hasAgent {
				return // 需要真实 Agent 的模板不在这里建图
			}
			if _, _, err := BuildStateGraphWithRegistry(context.Background(), cfg, NewRegistry(), nil); err != nil {
				t.Fatalf("template %s build: %v", tmpl.ID, err)
			}
		})
	}
}

// 内置 state.route:<字段>：按字段值选分支，字段为空时给出可读错误而不是乱走。
func TestStateRouteCondFunc(t *testing.T) {
	fn, err := NewRegistry().GetCondFunc(StateRouteCondFuncPrefix + "review_result")
	if err != nil {
		t.Fatal(err)
	}
	cond := fn.(trpcgraph.ConditionalFunc)
	if got, err := cond(context.Background(), trpcgraph.State{"review_result": " approved "}); err != nil || got != "approved" {
		t.Fatalf("got %q, %v", got, err)
	}
	if _, err := cond(context.Background(), trpcgraph.State{}); err == nil {
		t.Fatal("empty field should error")
	}
}

// 条件边没有条件函数：校验报错，建图返回错误而不是 panic。
func TestConditionalEdgeWithoutCondFunc(t *testing.T) {
	cfg := GraphBuildConfig{
		EntryPoint: "a",
		Nodes: []biz.NodeDef{
			{ID: "a", Type: "router"},
			{ID: "b", Type: "function", FuncRef: biz.SkipNodeFuncRef},
		},
		ConditionalEdges: []biz.ConditionalEdgeDef{{From: "a", PathMap: map[string]string{"x": "b"}}},
		FinishPoint:      "b",
	}
	res := ValidateGraph(context.Background(), &cfg, nil, NewRegistry())
	found := false
	for _, e := range res.Errors {
		found = found || (e.Code == ValidationErrCondFuncNotFound && e.NodeID == "a")
	}
	if !found {
		t.Fatalf("want cond_func_not_found on a, got %+v", res.Errors)
	}
	if _, _, err := BuildStateGraphWithRegistry(context.Background(), cfg, NewRegistry(), nil); err == nil {
		t.Fatal("build should fail without cond func")
	}
}

// 校验口径与建图口径一致：空函数引用报错，透传占位不报「未注册」。
func TestValidateNodeRefs_FunctionNodeRefs(t *testing.T) {
	cfg := GraphBuildConfig{
		EntryPoint:  "a",
		FinishPoint: "b",
		Nodes: []biz.NodeDef{
			{ID: "a", Type: "function"},
			{ID: "b", Type: "function", FuncRef: biz.SkipNodeFuncRef},
		},
		Edges: []EdgeDef{{From: "a", To: "b"}},
	}
	res := ValidateGraph(context.Background(), &cfg, nil, NewRegistry())
	var onA, onB int
	for _, e := range res.Errors {
		if e.Code != ValidationErrFuncRefNotFound {
			continue
		}
		switch e.NodeID {
		case "a":
			onA++
		case "b":
			onB++
		}
	}
	if onA != 1 || onB != 0 {
		t.Fatalf("func_ref errors: a=%d (want 1) b=%d (want 0); all=%+v", onA, onB, res.Errors)
	}
}
