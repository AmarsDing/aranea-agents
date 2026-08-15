package adapter

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
	graphtrpc "aranea-agents/internal/graph/trpc"
)

// TestReproCriticLoopStandaloneRun 复现 2026-08-15 事故：独立图（graph_definitions）
// 条件边 critic_loop_decision#2%verify（CriticLoopCondFuncRefForNode 生成的规范名）
// 经 BuildRuntime 路径应可解析（EnsureCriticLoopCondFuncs 按 cfg 注册参数化 ref）。
func TestReproCriticLoopStandaloneRun(t *testing.T) {
	reg := graphtrpc.NewRegistry()
	reg.RegisterNodeFuncInstance("noop", graphtrpc.PassthroughNodeFunc("noop"))
	f := NewGraphBuilderFactory(reg, nil, nil, nil, nil, graphtrpc.GraphNodeResolverSet{}, nil, nil)
	cfg := biz.GraphBuildConfig{
		EntryPoint: "verify",
		Nodes: []biz.NodeDef{
			{ID: "verify", Type: "function", FuncRef: "noop"},
			{ID: "postmortem", Type: "function", FuncRef: "noop"},
			{ID: "remediate", Type: "function", FuncRef: "noop"},
		},
		ConditionalEdges: []biz.ConditionalEdgeDef{
			{From: "verify", CondFuncRef: biz.CriticLoopCondFuncRefForNode(0, 2, "verify"), PathMap: map[string]string{
				"approved": "postmortem", "approved_forced": "postmortem", "retry": "remediate",
			}},
		},
	}
	if _, err := f.BuildRuntime(context.Background(), cfg, "s1", "s1", "g1", "e1", ""); err != nil {
		t.Fatalf("BuildRuntime failed: %v", err)
	}
}

// TestReproCriticLoopLegacyShortRefRejected 钉死命名边界：短名 "critic_loop#2%verify"
// 不是合法 ref（规范前缀为 critic_loop_decision），必须报 cond_func_not_found，
// 防止后续有人误以为短名可用而再踩 2026-08-15 的坑。
func TestReproCriticLoopLegacyShortRefRejected(t *testing.T) {
	reg := graphtrpc.NewRegistry()
	f := NewGraphBuilderFactory(reg, nil, nil, nil, nil, graphtrpc.GraphNodeResolverSet{}, nil, nil)
	cfg := biz.GraphBuildConfig{
		EntryPoint: "verify",
		Nodes:      []biz.NodeDef{{ID: "verify"}, {ID: "postmortem"}},
		ConditionalEdges: []biz.ConditionalEdgeDef{
			{From: "verify", CondFuncRef: "critic_loop#2%verify", PathMap: map[string]string{
				"approved": "postmortem", "retry": "verify",
			}},
		},
	}
	if _, err := f.BuildRuntime(context.Background(), cfg, "s1", "s1", "g1", "e1", ""); err == nil {
		t.Fatal("short ref critic_loop#2 must NOT resolve")
	}
}
