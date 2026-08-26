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

func TestOrchestrationSpecSourceRoundTrip(t *testing.T) {
	raw := `{"version":2,"mode":"sequential","source":"custom","members":[{"agent_id":"a1","role":"worker","enabled":true,"sort_order":1}]}`
	spec, err := ParseOrchestrationSpec(raw)
	if err != nil {
		t.Fatal(err)
	}
	if spec.Source != DefinitionGraphSourceCustom {
		t.Fatalf("source=%q, want custom", spec.Source)
	}
	out, err := OrchestrationSpecToDefinitionJSON(spec)
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(out), &body); err != nil {
		t.Fatal(err)
	}
	if body["source"] != "custom" {
		t.Fatalf("source lost on serialize: %v", body["source"])
	}
}

func TestOrchestrationSpecGraphSourceDefault(t *testing.T) {
	cases := map[string]string{
		"":                             DefinitionGraphSourcePreset,
		DefinitionGraphSourcePreset:    DefinitionGraphSourcePreset,
		DefinitionGraphSourceCustom:    DefinitionGraphSourceCustom,
		DefinitionGraphSourceLinkedExt: DefinitionGraphSourceLinkedExt,
		"unknown":                      DefinitionGraphSourcePreset,
	}
	for in, want := range cases {
		spec := OrchestrationSpec{Source: in}
		if got := spec.GraphSource(); got != want {
			t.Fatalf("GraphSource(%q)=%q, want %q", in, got, want)
		}
	}
}

// TestOrchestrationSpecDeliverableFieldsRoundTrip guards the deliverable
// channel fields against silent drops by the v2 canonical serialization
// (2026-08-07 根因：OrchestrationSpec 缺字段导致 materializeAndBind 重序列化
// 时丢弃 enable_state_deliverable/deliverable_contract/verification_gates，
// DAG 团队运行期无 set_deliverable 工具、闸门判失败、下游节点永不派发）。
func TestOrchestrationSpecDeliverableFieldsRoundTrip(t *testing.T) {
	raw := `{"version":2,"mode":"coordinator","runtime_engine":"graph","team_graph_runtime":true,` +
		`"members":[{"agent_id":"a1","role":"synthesizer","enabled":true,"sort_order":1},{"agent_id":"a2","role":"worker","enabled":true,"sort_order":2}],` +
		`"enable_state_deliverable":true,` +
		`"deliverable_contract":{"entries":[{"topic":"root_cause","description":"根因","required":true,"required_keys":["cause"]}]},` +
		`"verification_gates":[{"gate_type":"tool_assertion","description":"d","max_retries":3,"tool":"skill_list","assert_path":"enabled","assert_equals":"true"}]` +
		`}`
	spec, err := ParseOrchestrationSpec(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !spec.EnableStateDeliverable {
		t.Fatal("EnableStateDeliverable not parsed from definition JSON")
	}
	if spec.DeliverableContract == nil || len(spec.DeliverableContract.Entries) != 1 {
		t.Fatalf("DeliverableContract not parsed: %+v", spec.DeliverableContract)
	}
	if len(spec.VerificationGates) != 1 {
		t.Fatalf("VerificationGates not parsed: %+v", spec.VerificationGates)
	}
	out, err := OrchestrationSpecToDefinitionJSON(spec)
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(out), &body); err != nil {
		t.Fatal(err)
	}
	if body["enable_state_deliverable"] != true {
		t.Fatalf("enable_state_deliverable lost on canonical serialize: %v", body["enable_state_deliverable"])
	}
	contract, ok := body["deliverable_contract"].(map[string]any)
	if !ok {
		t.Fatalf("deliverable_contract lost on canonical serialize: %v", body["deliverable_contract"])
	}
	entries, ok := contract["entries"].([]any)
	if !ok || len(entries) != 1 {
		t.Fatalf("deliverable_contract.entries lost: %v", contract["entries"])
	}
	gates, ok := body["verification_gates"].([]any)
	if !ok || len(gates) != 1 {
		t.Fatalf("verification_gates lost on canonical serialize: %v", body["verification_gates"])
	}
}

// TestOrchestrationSpecDeliverableFieldsOmitWhenEmpty ensures the new fields
// stay absent (omitempty) when unset, so legacy JSON diffs stay clean.
func TestOrchestrationSpecDeliverableFieldsOmitWhenEmpty(t *testing.T) {
	out, err := OrchestrationSpecToDefinitionJSON(OrchestrationSpec{Mode: "sequential"})
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(out), &body); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"enable_state_deliverable", "deliverable_contract", "verification_gates"} {
		if _, ok := body[k]; ok {
			t.Fatalf("key %q must be omitted when unset, got %v", k, body[k])
		}
	}
}

// TestOrchestrationSpecEmptyContractNormalizedNil mirrors ParseDefinition's
// empty-entries → nil normalization so canonical JSON never carries
// deliverable_contract:{"entries":[]}.
func TestOrchestrationSpecEmptyContractNormalizedNil(t *testing.T) {
	spec := OrchestrationSpec{
		Mode:                   "sequential",
		DeliverableContract:    &MemberDeliverableContract{Entries: []MemberDeliverableEntry{}},
		EnableStateDeliverable: true,
	}
	out, err := OrchestrationSpecToDefinitionJSON(spec)
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(out), &body); err != nil {
		t.Fatal(err)
	}
	if _, ok := body["deliverable_contract"]; ok {
		t.Fatal("empty deliverable_contract must be normalized to nil (omitted)")
	}
	if body["enable_state_deliverable"] != true {
		t.Fatal("enable_state_deliverable must survive normalization")
	}
}

// TestOrchestrationSpecTokenBudgetRoundTrip pins 2026-08-26 M80 验收踩坑：
// token_budget_input_tokens 必须随 spec 往返（create/update 经
// OrchestrationSpecToDefinitionJSON 重序列化不得丢弃），且 0 值 omitempty
// 缺省时不出现。
func TestOrchestrationSpecTokenBudgetRoundTrip(t *testing.T) {
	spec, err := ParseOrchestrationSpec(
		`{"version":2,"mode":"sequential","members":[{"agent_id":"a1","role":"worker","enabled":true,"sort_order":0}],"token_budget_input_tokens":1}`)
	if err != nil {
		t.Fatal(err)
	}
	if spec.TokenBudgetInputTokens != 1 {
		t.Fatalf("parse lost budget: %+v", spec)
	}
	out, err := OrchestrationSpecToDefinitionJSON(spec)
	if err != nil {
		t.Fatal(err)
	}
	spec2, err := ParseOrchestrationSpec(out)
	if err != nil {
		t.Fatal(err)
	}
	if spec2.TokenBudgetInputTokens != 1 {
		t.Fatalf("canonical re-serialize dropped budget: %s", out)
	}

	// 零值（含 <0 关闭语义之外的缺省）omitempty 不出现，保持旧 JSON 干净。
	out, err = OrchestrationSpecToDefinitionJSON(OrchestrationSpec{Mode: "sequential"})
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(out), &body); err != nil {
		t.Fatal(err)
	}
	if _, ok := body["token_budget_input_tokens"]; ok {
		t.Fatalf("zero budget must be omitted, got %v", body["token_budget_input_tokens"])
	}
}
