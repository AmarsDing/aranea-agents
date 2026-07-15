package biz

import (
	"encoding/json"
	"testing"
)

func TestSpecV2RoundTrip(t *testing.T) {
	raw := `{"version":2,"mode":"parallel","runtime_engine":"graph","custom_field":42,"members":[{"agent_id":"a1","role":"worker","enabled":true,"sort_order":10}],"failure_policy":{"default":"retry_then_block","parallel_fail":"continue"}}`
	spec, err := ParseOrchestrationSpec(raw)
	if err != nil {
		t.Fatal(err)
	}
	if spec.RuntimeEngine != "graph" {
		t.Fatalf("runtime_engine=%q", spec.RuntimeEngine)
	}
	merged, err := MergeOrchestrationSpecIntoDefinition(raw, spec)
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(merged), &body); err != nil {
		t.Fatal(err)
	}
	if body["custom_field"] != float64(42) {
		t.Fatalf("custom_field lost: %v", body["custom_field"])
	}
}

func TestEnsureGraphRuntimeDefault(t *testing.T) {
	out := EnsureGraphRuntimeDefault(`{"version":1,"mode":"sequential","members":[]}`)
	spec, err := ParseOrchestrationSpec(out)
	if err != nil {
		t.Fatal(err)
	}
	if spec.RuntimeEngine != "graph" {
		t.Fatalf("expected graph default, got %q", spec.RuntimeEngine)
	}
}

func TestDefaultOrchestrationSpec(t *testing.T) {
	spec := DefaultOrchestrationSpec()
	if spec.RuntimeEngine != "graph" || spec.Version != OrchestrationSpecVersion {
		t.Fatalf("default spec=%+v", spec)
	}
}

func TestNormalizeBackfillMembersFromGraph(t *testing.T) {
	// Simulates data inconsistency: members empty but graph.nodes has agent nodes
	raw := `{"version":2,"mode":"sequential","members":[],"graph":{"version":1,"layout":"linear","nodes":[{"id":"start","type":"start","label":"开始"},{"id":"member-1","type":"agent","label":"代码审查员","agent_id":"abc123def456","role":"worker"},{"id":"member-2","type":"agent","label":"安全审计员","agent_id":"789ghi012jkl","role":"critic"},{"id":"end","type":"end","label":"结束"}],"edges":[]}}`
	spec, err := ParseOrchestrationSpec(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(spec.Members) != 2 {
		t.Fatalf("expected 2 members backfilled from graph.nodes, got %d", len(spec.Members))
	}
	if spec.Members[0].AgentID != "abc123def456" {
		t.Fatalf("member[0].AgentID=%q, want abc123def456", spec.Members[0].AgentID)
	}
	if spec.Members[0].Role != "worker" {
		t.Fatalf("member[0].Role=%q, want worker", spec.Members[0].Role)
	}
	if spec.Members[1].AgentID != "789ghi012jkl" {
		t.Fatalf("member[1].AgentID=%q, want 789ghi012jkl", spec.Members[1].AgentID)
	}
	if spec.Members[1].Role != "critic" {
		t.Fatalf("member[1].Role=%q, want critic", spec.Members[1].Role)
	}
	// Members already present should NOT be overwritten
	rawWithMembers := `{"version":2,"mode":"sequential","members":[{"agent_id":"existing","role":"coordinator","name":"已有","enabled":true,"sort_order":1}],"graph":{"version":1,"layout":"linear","nodes":[{"id":"member-1","type":"agent","label":"Graph Agent","agent_id":"fromgraph"}],"edges":[]}}`
	spec2, err := ParseOrchestrationSpec(rawWithMembers)
	if err != nil {
		t.Fatal(err)
	}
	if len(spec2.Members) != 1 {
		t.Fatalf("expected 1 member (not backfilled), got %d", len(spec2.Members))
	}
	if spec2.Members[0].AgentID != "existing" {
		t.Fatalf("member.AgentID=%q, want existing", spec2.Members[0].AgentID)
	}
}

func TestNormalizeBackfillPreservesTaskPromptAndEnabled(t *testing.T) {
	// Backfill from graph.nodes should preserve task_prompt and enabled fields
	raw := `{"version":2,"mode":"sequential","members":[],"graph":{"version":1,"layout":"linear","nodes":[{"id":"member-1","type":"agent","label":"审查员","agent_id":"abc123","role":"worker","task_prompt":"请审查代码","enabled":false}],"edges":[]}}`
	spec, err := ParseOrchestrationSpec(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(spec.Members) != 1 {
		t.Fatalf("expected 1 member, got %d", len(spec.Members))
	}
	if spec.Members[0].TaskPrompt != "请审查代码" {
		t.Fatalf("member.TaskPrompt=%q, want 请审查代码", spec.Members[0].TaskPrompt)
	}
	if spec.Members[0].Enabled() != false {
		t.Fatalf("member.Enabled()=%v, want false", spec.Members[0].Enabled())
	}
}

func TestEmbeddedGraphNodeSpec_RetryFallbackReviewerRoundTrip(t *testing.T) {
	// C-21: marshal→unmarshal preserves runtime node fields used by embedded graph.
	raw := `{
		"version":2,
		"mode":"sequential",
		"runtime_engine":"graph",
		"members":[],
		"graph":{
			"version":1,
			"layout":"linear",
			"nodes":[
				{
					"id":"n1",
					"type":"agent",
					"label":"Worker",
					"agent_id":"a1",
					"retry_max_attempts":3,
					"fallback_agent":"a2",
					"reviewer_agent":"critic",
					"review_rules":"approve if tests pass",
					"func_ref":"pkg.Fn"
				}
			],
			"edges":[]
		}
	}`
	spec, err := ParseOrchestrationSpec(raw)
	if err != nil {
		t.Fatal(err)
	}
	if spec.Graph == nil || len(spec.Graph.Nodes) != 1 {
		t.Fatalf("graph nodes=%v", spec.Graph)
	}
	n := spec.Graph.Nodes[0]
	if n.RetryMaxAttempts != 3 || n.FallbackAgent != "a2" || n.ReviewerAgent != "critic" ||
		n.ReviewRules != "approve if tests pass" || n.FuncRef != "pkg.Fn" {
		t.Fatalf("parse lost node fields: %+v", n)
	}
	out, err := OrchestrationSpecToDefinitionJSON(spec)
	if err != nil {
		t.Fatal(err)
	}
	spec2, err := ParseOrchestrationSpec(out)
	if err != nil {
		t.Fatal(err)
	}
	n2 := spec2.Graph.Nodes[0]
	if n2.RetryMaxAttempts != 3 || n2.FallbackAgent != "a2" || n2.ReviewerAgent != "critic" ||
		n2.ReviewRules != "approve if tests pass" || n2.FuncRef != "pkg.Fn" {
		t.Fatalf("round-trip lost node fields: %+v", n2)
	}

	// Partial overlay (proto-shaped: missing runtime fields) must not wipe base JSON.
	partial := OrchestrationSpec{
		Version: 2,
		Mode:    "sequential",
		Graph: &EmbeddedGraphSpec{
			Version: 1,
			Layout:  "linear",
			Nodes: []EmbeddedGraphNodeSpec{{
				ID: "n1", Type: "agent", Label: "Worker", AgentID: "a1",
			}},
		},
	}
	merged, err := MergeOrchestrationSpecIntoDefinition(raw, partial)
	if err != nil {
		t.Fatal(err)
	}
	spec3, err := ParseOrchestrationSpec(merged)
	if err != nil {
		t.Fatal(err)
	}
	n3 := spec3.Graph.Nodes[0]
	if n3.RetryMaxAttempts != 3 || n3.FallbackAgent != "a2" || n3.ReviewerAgent != "critic" {
		t.Fatalf("merge wiped runtime fields: %+v", n3)
	}
}

func TestMergeProtectsBaseMembers(t *testing.T) {
	// B1 fix: overlay with empty members should not overwrite base members
	base := `{"version":2,"mode":"sequential","members":[{"agent_id":"existing","role":"coordinator","name":"已有","enabled":true,"sort_order":1}]}`
	spec := OrchestrationSpec{Version: 2, Mode: "sequential", Members: []OrchestrationMember{}}
	merged, err := MergeOrchestrationSpecIntoDefinition(base, spec)
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(merged), &body); err != nil {
		t.Fatal(err)
	}
	members, ok := body["members"].([]any)
	if !ok || len(members) != 1 {
		t.Fatalf("base members should be preserved when overlay members is empty, got %v", body["members"])
	}
}
