package biz

import "testing"

func TestApplyFailurePolicy_defaultRetry(t *testing.T) {
	cfg := GraphBuildConfig{
		Nodes: []NodeDef{{ID: "member-1", Type: "agent"}, {ID: "member-2", Type: "agent"}},
	}
	policy := &TeamFailurePolicy{Default: FailureDefaultRetryThenBlock, Retry: TeamRetryPolicy{MaxAttempts: 5}}
	out := ApplyFailurePolicy(cfg, policy)
	if out.Nodes[0].RetryMaxAttempts != 5 {
		t.Fatalf("node0 retry=%d want 5", out.Nodes[0].RetryMaxAttempts)
	}
	if out.Nodes[0].FailureAction != FailureDefaultRetryThenBlock {
		t.Fatalf("action=%q", out.Nodes[0].FailureAction)
	}
}

func TestApplyFailurePolicy_nodeOverrideSkip(t *testing.T) {
	cfg := GraphBuildConfig{Nodes: []NodeDef{{ID: "member-1", Type: "agent"}}}
	policy := &TeamFailurePolicy{
		Default: FailureDefaultRetryThenBlock,
		Retry:   TeamRetryPolicy{MaxAttempts: 2},
		NodeOverrides: map[string]TeamNodeFailureOverride{
			"member-1": {Policy: FailureDefaultSkip, FallbackAgent: "backup-key"},
		},
	}
	out := ApplyFailurePolicy(cfg, policy)
	if out.Nodes[0].FailureAction != FailureDefaultSkip {
		t.Fatalf("action=%q want skip", out.Nodes[0].FailureAction)
	}
	if out.Nodes[0].RetryMaxAttempts != 0 {
		t.Fatalf("skip node should not get retry")
	}
	if out.Nodes[0].FallbackAgent != "backup-key" {
		t.Fatalf("fallback=%q", out.Nodes[0].FallbackAgent)
	}
}

func TestEnsureFailureRecoveryStateFields(t *testing.T) {
	cfg := GraphBuildConfig{
		Nodes: []NodeDef{{ID: "member-1", Type: "agent", FailureAction: FailureOnFailureSkip}},
	}
	out := EnsureFailureRecoveryStateFields(cfg)
	if len(out.StateFields) != 1 || out.StateFields[0].Name != SkippedNodesStateKey {
		t.Fatalf("fields=%v", out.StateFields)
	}
}

func TestParallelBranchNodeIDs_parallelJoin(t *testing.T) {
	cfg := GraphBuildConfig{
		FinishPoint: "member-3",
		Edges: []EdgeDef{
			{From: "member-1", To: "member-2"},
			{From: "member-1", To: "member-3"},
			{From: "member-2", To: "member-3"},
		},
	}
	branches := parallelBranchNodeIDs(cfg)
	if len(branches) != 2 {
		t.Fatalf("branches=%v", branches)
	}
	if _, ok := branches["member-1"]; !ok {
		t.Fatal("missing member-1")
	}
	if _, ok := branches["member-2"]; !ok {
		t.Fatal("missing member-2")
	}
}

func TestApplyParallelFailContinue(t *testing.T) {
	cfg := GraphBuildConfig{
		FinishPoint: "member-3",
		Edges: []EdgeDef{
			{From: "member-1", To: "member-2"},
			{From: "member-1", To: "member-3"},
			{From: "member-2", To: "member-3"},
		},
		Nodes: []NodeDef{
			{ID: "member-1", Type: "agent", FailureAction: FailureDefaultRetryThenBlock, RetryMaxAttempts: 2},
			{ID: "member-2", Type: "agent", FailureAction: FailureDefaultRetryThenBlock, RetryMaxAttempts: 2},
			{ID: "member-3", Type: "agent", FailureAction: FailureDefaultRetryThenBlock},
		},
		FailurePolicy: &TeamFailurePolicy{ParallelFail: ParallelFailContinue},
	}
	out := ApplyParallelFailContinue(cfg)
	if out.Nodes[0].FailureAction != FailureOnFailureSkip {
		t.Fatalf("member-1 action=%q", out.Nodes[0].FailureAction)
	}
	if out.Nodes[1].FailureAction != FailureOnFailureSkip {
		t.Fatalf("member-2 action=%q", out.Nodes[1].FailureAction)
	}
	if out.Nodes[0].RetryMaxAttempts != 2 {
		t.Fatalf("member-1 retry should be preserved")
	}
	if out.Nodes[2].FailureAction != FailureDefaultRetryThenBlock {
		t.Fatalf("finish node should not change")
	}
}

func TestApplyParallelFailContinue_explicitBranchIDs(t *testing.T) {
	cfg := GraphBuildConfig{
		FinishPoint:       "member-3",
		ParallelBranchIDs: []string{"member-1", "member-2"},
		Nodes: []NodeDef{
			{ID: "member-1", Type: "agent", FailureAction: FailureDefaultRetryThenBlock},
			{ID: "member-2", Type: "agent", FailureAction: FailureDefaultRetryThenBlock},
			{ID: "member-3", Type: "agent", FailureAction: FailureDefaultRetryThenBlock},
		},
		FailurePolicy: &TeamFailurePolicy{ParallelFail: ParallelFailContinue},
	}
	out := ApplyParallelFailContinue(cfg)
	if out.Nodes[0].FailureAction != FailureOnFailureSkip {
		t.Fatalf("member-1 action=%q", out.Nodes[0].FailureAction)
	}
	if out.Nodes[1].FailureAction != FailureOnFailureSkip {
		t.Fatalf("member-2 action=%q", out.Nodes[1].FailureAction)
	}
}

func TestFilterVisualizationEdges(t *testing.T) {
	cfg := GraphBuildConfig{
		Edges: []EdgeDef{
			{From: "a", To: "b", Kind: "flow"},
			{From: "a", To: "c", Kind: "transfer"},
		},
	}
	out := FilterVisualizationEdges(cfg)
	if len(out.Edges) != 1 || out.Edges[0].To != "b" {
		t.Fatalf("edges=%v", out.Edges)
	}
}
