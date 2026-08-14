package team

import (
	"testing"

	"aranea-agents/internal/biz"
)

// P2-4：enable_project_state 开启时 graph schema 注入 project_state
// StateField（MergeReducer），成员工具集携带 update_project_state；关闭时零注入。
func TestFinalizeRuntimeGraphConfig_projectStateField(t *testing.T) {
	cfg := biz.GraphBuildConfig{
		Nodes:       []biz.NodeDef{{ID: "member-1", Type: "agent"}},
		EntryPoint:  "member-1",
		FinishPoint: "member-1",
		Edges:       []biz.EdgeDef{{From: "member-1", To: "end"}},
	}

	// Case 1: EnableProjectState=true → StateFields contains project_state
	def := Definition{
		Mode:               "sequential",
		EnableProjectState: true,
		Members:            []MemberDef{{AgentID: "a", SortOrder: 1}},
	}
	out := finalizeRuntimeGraphConfig(cfg, def, `{"mode":"sequential","enable_project_state":true}`, nil, nil)
	found := false
	for _, sf := range out.StateFields {
		if sf.Name == biz.ProjectStateKey {
			found = true
			if sf.Reducer != biz.ReducerMerge {
				t.Fatalf("project_state reducer=%q want %q", sf.Reducer, biz.ReducerMerge)
			}
			if sf.Type != "map[string]any" {
				t.Fatalf("project_state type=%q want map[string]any", sf.Type)
			}
		}
	}
	if !found {
		t.Fatalf("expected StateFields to contain %q, got: %#v", biz.ProjectStateKey, out.StateFields)
	}

	// Case 2: EnableProjectState=false → no project_state StateField
	def2 := Definition{Mode: "sequential", Members: []MemberDef{{AgentID: "a", SortOrder: 1}}}
	out2 := finalizeRuntimeGraphConfig(cfg, def2, `{"mode":"sequential"}`, nil, nil)
	for _, sf := range out2.StateFields {
		if sf.Name == biz.ProjectStateKey {
			t.Fatalf("expected no project_state StateField when disabled, got: %#v", sf)
		}
	}

	// Case 3: Idempotent — 二次注入不重复。
	out3 := finalizeRuntimeGraphConfig(out, def, `{"mode":"sequential","enable_project_state":true}`, nil, nil)
	count := 0
	for _, sf := range out3.StateFields {
		if sf.Name == biz.ProjectStateKey {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("project_state field count=%d, want exactly 1 (idempotent)", count)
	}
}

func TestProjectStateToolsForDef(t *testing.T) {
	if tools := projectStateToolsForDef(Definition{}); tools != nil {
		t.Fatalf("disabled def must yield nil tools, got %d", len(tools))
	}
	tools := projectStateToolsForDef(Definition{EnableProjectState: true})
	if len(tools) != 1 || tools[0].Declaration().Name != "update_project_state" {
		names := make([]string, 0, len(tools))
		for _, tl := range tools {
			names = append(names, tl.Declaration().Name)
		}
		t.Fatalf("tools=%v, want exactly [update_project_state]", names)
	}
}
