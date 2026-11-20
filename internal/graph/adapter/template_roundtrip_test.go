package adapter

import (
	"testing"

	"aranea-agents/internal/biz"
	graphtrpc "aranea-agents/internal/graph/trpc"
)

// 模板经 biz 层转一圈再建定义：条件边的 state.route 引用不能丢（此前丢了，审批流等建出来就跑不了）。
func TestTemplateToDef_KeepsRouteCondFunc(t *testing.T) {
	f := &trpcGraphBuilderFactory{}
	for _, id := range []string{"approval", "review_loop", "dispatch"} {
		ref, ok := f.GetTemplate(id)
		if !ok {
			t.Fatalf("template %s missing", id)
		}
		def := f.TemplateToDef(ref, "x", "")
		if len(def.ConditionalEdges) == 0 {
			t.Fatalf("%s: no conditional edges", id)
		}
		for _, ce := range def.ConditionalEdges {
			if len(ce.CondFuncRef) <= len(graphtrpc.StateRouteCondFuncPrefix) {
				t.Fatalf("%s: cond edge from %s has ref %q", id, ce.From, ce.CondFuncRef)
			}
		}
		for _, n := range def.Nodes {
			if n.Type == biz.NodeTypeFunction && n.FuncRef == "" {
				t.Fatalf("%s: function node %s without func ref", id, n.ID)
			}
		}
	}
}
