package service

import (
	"encoding/json"
	"testing"

	v1 "aranea-agents/api/kratos/team/v1"
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
