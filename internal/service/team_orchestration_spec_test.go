package service

import (
	"encoding/json"
	"testing"

	v1 "aranea-agents/api/kratos/team/v1"
	"aranea-agents/internal/biz"
)

func TestMergeTeamDefinitionFromRequest_orchestrationSpec(t *testing.T) {
	base := `{"version":1,"mode":"sequential","custom":true,"members":[]}`
	pb := &v1.Team{
		OrchestrationSpec: &v1.OrchestrationSpec{
			Version:       2,
			Mode:          "parallel",
			RuntimeEngine: "graph",
			FailurePolicy: &v1.FailurePolicySpec{OnError: "halt"},
		},
		LinkedGraphId: "g-1",
	}
	merged, err := mergeTeamDefinitionFromRequest(base, pb)
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(merged), &body); err != nil {
		t.Fatal(err)
	}
	if body["custom"] != true {
		t.Fatalf("custom field lost: %v", body["custom"])
	}
	if body["mode"] != "parallel" {
		t.Fatalf("mode=%v", body["mode"])
	}
	if body["linked_graph_id"] != "g-1" {
		t.Fatalf("linked_graph_id=%v", body["linked_graph_id"])
	}
	fp, ok := body["failure_policy"].(map[string]any)
	if !ok || fp["on_error"] != "halt" {
		t.Fatalf("failure_policy=%v", body["failure_policy"])
	}
}

func TestFromProtoOrchestrationSpec_defaultsGraph(t *testing.T) {
	spec := fromProtoOrchestrationSpec(&v1.OrchestrationSpec{Version: 2, RuntimeEngine: "graph"})
	if spec.RuntimeEngine != "graph" {
		t.Fatalf("runtime=%q", spec.RuntimeEngine)
	}
}

// TestOrchestrationSpec_EmbeddedGraphNodeFields_ProtoGoldenRoundTrip ensures
// C-21 runtime node fields survive proto -> biz mapping (API read->write).
func TestOrchestrationSpec_EmbeddedGraphNodeFields_ProtoGoldenRoundTrip(t *testing.T) {
	enabled := true
	in := &v1.OrchestrationSpec{
		Version:       2,
		Mode:          "sequential",
		RuntimeEngine: "graph",
		Graph: &v1.EmbeddedGraph{
			Version: 1,
			Layout:  "linear",
			Nodes: []*v1.EmbeddedGraphNode{{
				Id:               "n1",
				Type:             "agent",
				Label:            "Worker",
				AgentId:          "a1",
				Enabled:          &enabled,
				RetryMaxAttempts: 3,
				FallbackAgent:    "a2",
				ReviewerAgent:    "critic",
				ReviewRules:      "approve if tests pass",
				FuncRef:          "pkg.Fn",
			}},
		},
	}

	bizSpec := fromProtoOrchestrationSpec(in)
	if bizSpec.Graph == nil || len(bizSpec.Graph.Nodes) != 1 {
		t.Fatalf("biz graph nodes=%v", bizSpec.Graph)
	}
	n := bizSpec.Graph.Nodes[0]
	if n.RetryMaxAttempts != 3 || n.FallbackAgent != "a2" || n.ReviewerAgent != "critic" ||
		n.ReviewRules != "approve if tests pass" || n.FuncRef != "pkg.Fn" {
		t.Fatalf("fromProto lost fields: %+v", n)
	}

	raw, err := biz.OrchestrationSpecToDefinitionJSON(bizSpec)
	if err != nil {
		t.Fatal(err)
	}
	out := toProtoOrchestrationSpec(raw)
	if out == nil || out.Graph == nil || len(out.Graph.Nodes) != 1 {
		t.Fatalf("toProto graph=%v", out)
	}
	pn := out.Graph.Nodes[0]
	if pn.GetRetryMaxAttempts() != 3 || pn.GetFallbackAgent() != "a2" ||
		pn.GetReviewerAgent() != "critic" || pn.GetReviewRules() != "approve if tests pass" ||
		pn.GetFuncRef() != "pkg.Fn" {
		t.Fatalf("toProto lost fields: %+v", pn)
	}
}
