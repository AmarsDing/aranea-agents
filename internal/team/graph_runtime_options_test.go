package team

import (
	"testing"

	"aranea-agents/internal/biz"
)

func TestApplyTeamRuntimeExecutionOptions_checkpointDefault(t *testing.T) {
	def := Definition{Mode: "sequential"}
	cfg := biz.GraphBuildConfig{Nodes: []biz.NodeDef{{ID: "member-1", Type: "agent"}}}
	out := applyTeamRuntimeExecutionOptions(cfg, def, `{"mode":"sequential"}`)
	if !out.EnableCheckpoint {
		t.Fatal("expected checkpoint enabled by default")
	}
	out = applyTeamRuntimeExecutionOptions(cfg, def, `{"enable_checkpoint":false}`)
	if out.EnableCheckpoint {
		t.Fatal("expected checkpoint disabled when spec says false")
	}
}

func TestParseRuntimeOptions(t *testing.T) {
	opts := parseRuntimeOptions("")
	if !opts.EnableCheckpoint || !opts.CheckpointPresent {
		t.Fatal("empty JSON should default to checkpoint enabled")
	}
	opts = parseRuntimeOptions(`{"enable_checkpoint":false,"graph":{"nodes":[{"id":"a","interrupt_before":true}]}}`)
	if opts.EnableCheckpoint {
		t.Fatal("expected checkpoint disabled")
	}
	if len(opts.Nodes) != 1 || opts.Nodes[0].ID != "a" {
		t.Fatalf("nodes=%v", opts.Nodes)
	}
	opts = parseRuntimeOptions(`invalid json`)
	if !opts.EnableCheckpoint || opts.CheckpointPresent {
		t.Fatal("invalid JSON should default to checkpoint enabled with CheckpointPresent=false")
	}
}

func TestCollectGraphInterrupts(t *testing.T) {
	raw := `{"graph":{"nodes":[{"id":"a","interrupt_before":true},{"id":"b","interrupt_after":true}]}}`
	before, after := collectGraphInterrupts(raw)
	if len(before) != 1 || before[0] != "a" {
		t.Fatalf("before=%v", before)
	}
	if len(after) != 1 || after[0] != "b" {
		t.Fatalf("after=%v", after)
	}
}

func TestApplyEmbeddedNodePolicies(t *testing.T) {
	cfg := &biz.GraphBuildConfig{
		Nodes: []biz.NodeDef{{ID: "router-1", Type: "router"}},
	}
	raw := `{"graph":{"nodes":[{"id":"router-1","destinations":["member-2"],"retry_max_attempts":4,"fallback_agent":"backup"}]}}`
	applyEmbeddedNodePolicies(cfg, raw)
	n := cfg.Nodes[0]
	if len(n.Destinations) != 1 || n.Destinations[0] != "member-2" {
		t.Fatalf("destinations=%v", n.Destinations)
	}
	if n.RetryMaxAttempts != 4 {
		t.Fatalf("retry=%d", n.RetryMaxAttempts)
	}
	if n.FallbackAgent != "backup" {
		t.Fatalf("fallback=%q", n.FallbackAgent)
	}
}

func TestApplyTeamRuntimeExecutionOptions_circuitBreaker(t *testing.T) {
	def := Definition{
		FailurePolicy: &FailurePolicy{
			CircuitBreaker: &CircuitBreakerPolicyDef{FailureThreshold: 3},
		},
	}
	cfg := biz.GraphBuildConfig{
		Nodes: []biz.NodeDef{{ID: "member-1", Type: "agent"}},
	}
	out := applyTeamRuntimeExecutionOptions(cfg, def, `{}`)
	if out.Nodes[0].RetryMaxAttempts != 1 {
		t.Fatalf("expected retry floor for breaker, got %d", out.Nodes[0].RetryMaxAttempts)
	}
}

func TestFinalizeRuntimeGraphConfig_deliverableStateField(t *testing.T) {
	// Case 1: EnableStateDeliverable=true → StateFields contains deliverable
	def := Definition{
		Mode:                    "sequential",
		EnableStateDeliverable:  true,
		Members:                 []MemberDef{{AgentID: "a", SortOrder: 1}},
	}
	cfg := biz.GraphBuildConfig{
		Nodes:       []biz.NodeDef{{ID: "member-1", Type: "agent"}},
		EntryPoint:  "member-1",
		FinishPoint: "member-1",
		Edges:       []biz.EdgeDef{{From: "member-1", To: "end"}},
	}
	out := finalizeRuntimeGraphConfig(cfg, def, `{"mode":"sequential","enable_state_deliverable":true}`, nil, nil)
	found := false
	for _, sf := range out.StateFields {
		if sf.Name == biz.DeliverableStateKey {
			found = true
			if sf.Reducer != biz.ReducerCover {
				t.Fatalf("deliverable reducer=%q want %q", sf.Reducer, biz.ReducerCover)
			}
			if sf.Type != "map[string]any" {
				t.Fatalf("deliverable type=%q want map[string]any", sf.Type)
			}
		}
	}
	if !found {
		t.Fatalf("expected StateFields to contain %q, got: %#v", biz.DeliverableStateKey, out.StateFields)
	}

	// Case 2: EnableStateDeliverable=false → no deliverable StateField
	def2 := Definition{Mode: "sequential", Members: []MemberDef{{AgentID: "a", SortOrder: 1}}}
	out2 := finalizeRuntimeGraphConfig(cfg, def2, `{"mode":"sequential"}`, nil, nil)
	for _, sf := range out2.StateFields {
		if sf.Name == biz.DeliverableStateKey {
			t.Fatalf("expected no deliverable StateField when disabled, got: %#v", sf)
		}
	}

	// Case 3: Idempotent — calling twice does not duplicate
	out3 := finalizeRuntimeGraphConfig(out, def, `{"mode":"sequential","enable_state_deliverable":true}`, nil, nil)
	count := 0
	for _, sf := range out3.StateFields {
		if sf.Name == biz.DeliverableStateKey {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected exactly 1 deliverable StateField after double finalize, got %d", count)
	}
}
