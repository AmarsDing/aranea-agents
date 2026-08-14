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
	branches := parallelBranchNodeIDs(cfg, nil)
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
	}
	policy := &TeamFailurePolicy{ParallelFail: ParallelFailContinue}
	out := ApplyParallelFailContinue(cfg, policy, nil)
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
		FinishPoint: "member-3",
		Nodes: []NodeDef{
			{ID: "member-1", Type: "agent", FailureAction: FailureDefaultRetryThenBlock},
			{ID: "member-2", Type: "agent", FailureAction: FailureDefaultRetryThenBlock},
			{ID: "member-3", Type: "agent", FailureAction: FailureDefaultRetryThenBlock},
		},
	}
	policy := &TeamFailurePolicy{ParallelFail: ParallelFailContinue}
	branchIDs := []string{"member-1", "member-2"}
	out := ApplyParallelFailContinue(cfg, policy, branchIDs)
	if out.Nodes[0].FailureAction != FailureOnFailureSkip {
		t.Fatalf("member-1 action=%q", out.Nodes[0].FailureAction)
	}
	if out.Nodes[1].FailureAction != FailureOnFailureSkip {
		t.Fatalf("member-2 action=%q", out.Nodes[1].FailureAction)
	}
}

// TestApplyParallelFailContinue_deepBranchIntermediateNodes (Y8)：深度≥3 的
// 分支（fan-out 后经多跳才汇入 join）中，分支中间节点也必须标记
// skip-on-failure。否则中间节点失败按默认 retry_then_block 传播，直接中止
// 整个 run——parallel_fail=continue 形同虚设。branchIDs 口径与编译期
// compileEmbeddedEdges 一致：join feeders（分支尾节点）。
func TestApplyParallelFailContinue_deepBranchIntermediateNodes(t *testing.T) {
	cfg := GraphBuildConfig{
		FinishPoint: "synth",
		Nodes: []NodeDef{
			{ID: "a1", Type: "agent", FailureAction: FailureDefaultRetryThenBlock},
			{ID: "a2", Type: "agent", FailureAction: FailureDefaultRetryThenBlock},
			{ID: "b1", Type: "agent", FailureAction: FailureDefaultRetryThenBlock},
			{ID: "synth", Type: "agent", FailureAction: FailureDefaultRetryThenBlock},
		},
		Edges: []EdgeDef{
			{From: "a1", To: "a2"},
			{From: "a2", To: "synth"},
			{From: "b1", To: "synth"},
		},
	}
	policy := &TeamFailurePolicy{ParallelFail: ParallelFailContinue}
	out := ApplyParallelFailContinue(cfg, policy, []string{"a2", "b1"})
	marked := map[string]string{}
	for _, n := range out.Nodes {
		marked[n.ID] = n.FailureAction
	}
	for _, id := range []string{"a1", "a2", "b1"} {
		if marked[id] != FailureOnFailureSkip {
			t.Fatalf("%s action=%q want skip_on_failure（分支全链路标记）", id, marked[id])
		}
	}
	if marked["synth"] != FailureDefaultRetryThenBlock {
		t.Fatalf("finish 节点不可标记 skip-on-failure: %q", marked["synth"])
	}
}

// TestApplyParallelFailContinue_sharedUpstreamNotMarked (Y8 边界)：可同时
// 到达 ≥2 个 join feeder 的共享上游节点（fan-out 入口）不得标记
// skip-on-failure——它承载所有分支，失败必须中止全局而非静默吞掉。
func TestApplyParallelFailContinue_sharedUpstreamNotMarked(t *testing.T) {
	cfg := GraphBuildConfig{
		FinishPoint: "synth",
		Nodes: []NodeDef{
			{ID: "entry", Type: "agent", FailureAction: FailureDefaultRetryThenBlock},
			{ID: "a1", Type: "agent", FailureAction: FailureDefaultRetryThenBlock},
			{ID: "b1", Type: "agent", FailureAction: FailureDefaultRetryThenBlock},
			{ID: "synth", Type: "agent", FailureAction: FailureDefaultRetryThenBlock},
		},
		Edges: []EdgeDef{
			{From: "entry", To: "a1"},
			{From: "entry", To: "b1"},
			{From: "a1", To: "synth"},
			{From: "b1", To: "synth"},
		},
	}
	policy := &TeamFailurePolicy{ParallelFail: ParallelFailContinue}
	out := ApplyParallelFailContinue(cfg, policy, []string{"a1", "b1"})
	marked := map[string]string{}
	for _, n := range out.Nodes {
		marked[n.ID] = n.FailureAction
	}
	if marked["entry"] != FailureDefaultRetryThenBlock {
		t.Fatalf("共享上游 entry 不可标记 skip-on-failure: %q", marked["entry"])
	}
	for _, id := range []string{"a1", "b1"} {
		if marked[id] != FailureOnFailureSkip {
			t.Fatalf("%s action=%q want skip_on_failure", id, marked[id])
		}
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

func TestApplyFailurePolicy_nil(t *testing.T) {
	cfg := GraphBuildConfig{Nodes: []NodeDef{{ID: "n1"}}}
	out := ApplyFailurePolicy(cfg, nil)
	if len(out.Nodes) != 1 || out.Nodes[0].ID != "n1" {
		t.Fatalf("nil policy should pass through: %+v", out)
	}
}

func TestApplyFailurePolicy_defaultRetryAutoMaxAttempts(t *testing.T) {
	cfg := GraphBuildConfig{Nodes: []NodeDef{{ID: "n1", Type: "agent"}}}
	policy := &TeamFailurePolicy{Default: FailureDefaultRetryThenBlock, Retry: TeamRetryPolicy{}}
	out := ApplyFailurePolicy(cfg, policy)
	if out.Nodes[0].RetryMaxAttempts != 3 {
		t.Fatalf("auto max_attempts=%d want 3", out.Nodes[0].RetryMaxAttempts)
	}
}

func TestApplyFailurePolicy_skipNoRetry(t *testing.T) {
	cfg := GraphBuildConfig{Nodes: []NodeDef{{ID: "n1", Type: "agent"}}}
	policy := &TeamFailurePolicy{Default: FailureDefaultSkip}
	out := ApplyFailurePolicy(cfg, policy)
	if out.Nodes[0].FailureAction != FailureDefaultSkip {
		t.Fatalf("action=%q want skip", out.Nodes[0].FailureAction)
	}
	if out.Nodes[0].RetryMaxAttempts != 0 {
		t.Fatalf("skip node should not get retry attempts")
	}
}

func TestApplyFailurePolicy_overrideRetry(t *testing.T) {
	cfg := GraphBuildConfig{Nodes: []NodeDef{{ID: "n1", Type: "agent"}}}
	policy := &TeamFailurePolicy{
		Default: FailureDefaultRetryThenBlock,
		Retry:   TeamRetryPolicy{MaxAttempts: 2},
		NodeOverrides: map[string]TeamNodeFailureOverride{
			"n1": {Retry: &TeamRetryPolicy{MaxAttempts: 10}},
		},
	}
	out := ApplyFailurePolicy(cfg, policy)
	if out.Nodes[0].RetryMaxAttempts != 10 {
		t.Fatalf("override retry=%d want 10", out.Nodes[0].RetryMaxAttempts)
	}
}

func TestApplySkipNodeSemanticsExtended(t *testing.T) {
	t.Run("converts_skip_nodes", func(t *testing.T) {
		cfg := GraphBuildConfig{
			Nodes: []NodeDef{
				{ID: "n1", Type: "agent", FailureAction: FailureDefaultSkip, AgentName: "worker"},
				{ID: "n2", Type: "agent", FailureAction: FailureDefaultRetryThenBlock},
			},
		}
		out := ApplySkipNodeSemantics(cfg)
		if out.Nodes[0].Type != "function" {
			t.Fatalf("type=%q want function", out.Nodes[0].Type)
		}
		if out.Nodes[0].FuncRef != SkipNodeFuncRef {
			t.Fatalf("func_ref=%q want %q", out.Nodes[0].FuncRef, SkipNodeFuncRef)
		}
		if out.Nodes[0].AgentName != "" {
			t.Fatalf("agent_name should be cleared for skip node")
		}
		if out.Nodes[1].Type != "agent" {
			t.Fatalf("non-skip node should be unchanged: type=%q", out.Nodes[1].Type)
		}
	})

	t.Run("adds_state_field", func(t *testing.T) {
		cfg := GraphBuildConfig{
			Nodes: []NodeDef{{ID: "n1", Type: "agent", FailureAction: FailureDefaultSkip}},
		}
		out := ApplySkipNodeSemantics(cfg)
		found := false
		for _, sf := range out.StateFields {
			if sf.Name == SkippedNodesStateKey {
				found = true
				if sf.Reducer != ReducerAppend {
					t.Fatalf("reducer=%q want append", sf.Reducer)
				}
			}
		}
		if !found {
			t.Fatal("expected _skipped_nodes state field")
		}
	})

	t.Run("no_skip_nodes", func(t *testing.T) {
		cfg := GraphBuildConfig{
			Nodes: []NodeDef{{ID: "n1", Type: "agent", FailureAction: FailureDefaultRetryThenBlock}},
		}
		out := ApplySkipNodeSemantics(cfg)
		if out.Nodes[0].Type != "agent" {
			t.Fatalf("non-skip node should not be modified")
		}
		if len(out.StateFields) != 0 {
			t.Fatalf("no state fields should be added: %v", out.StateFields)
		}
	})
}

func TestApplyCircuitBreakerPolicy(t *testing.T) {
	t.Run("nil_policy", func(t *testing.T) {
		cfg := GraphBuildConfig{Nodes: []NodeDef{{ID: "n1", Type: "agent"}}}
		out := ApplyCircuitBreakerPolicy(cfg, nil)
		if out.Nodes[0].RetryMaxAttempts != 0 {
			t.Fatalf("nil policy should not modify nodes")
		}
	})

	t.Run("zero_threshold", func(t *testing.T) {
		cfg := GraphBuildConfig{Nodes: []NodeDef{{ID: "n1", Type: "agent"}}}
		out := ApplyCircuitBreakerPolicy(cfg, &CircuitBreakerPolicy{FailureThreshold: 0})
		if out.Nodes[0].RetryMaxAttempts != 0 {
			t.Fatalf("zero threshold should not modify nodes")
		}
	})

	t.Run("sets_min_retry", func(t *testing.T) {
		cfg := GraphBuildConfig{
			Nodes: []NodeDef{
				{ID: "n1", Type: "agent"},
				{ID: "n2", Type: "llm"},
				{ID: "n3", Type: "tool"},
				{ID: "n4", Type: "router"},
			},
		}
		policy := &CircuitBreakerPolicy{FailureThreshold: 3, ResetTimeoutSeconds: 60}
		out := ApplyCircuitBreakerPolicy(cfg, policy)
		if out.CircuitBreaker == nil || out.CircuitBreaker.FailureThreshold != 3 {
			t.Fatalf("CircuitBreaker not attached: %+v", out.CircuitBreaker)
		}
		if out.Nodes[0].RetryMaxAttempts != 1 {
			t.Fatalf("agent retry=%d want 1", out.Nodes[0].RetryMaxAttempts)
		}
		if out.Nodes[1].RetryMaxAttempts != 1 {
			t.Fatalf("llm retry=%d want 1", out.Nodes[1].RetryMaxAttempts)
		}
		if out.Nodes[2].RetryMaxAttempts != 1 {
			t.Fatalf("tool retry=%d want 1", out.Nodes[2].RetryMaxAttempts)
		}
		if out.Nodes[3].RetryMaxAttempts != 0 {
			t.Fatalf("router should not get retry: %d", out.Nodes[3].RetryMaxAttempts)
		}
	})

	t.Run("preserves_existing_retry", func(t *testing.T) {
		cfg := GraphBuildConfig{
			Nodes: []NodeDef{{ID: "n1", Type: "agent", RetryMaxAttempts: 5}},
		}
		policy := &CircuitBreakerPolicy{FailureThreshold: 3}
		out := ApplyCircuitBreakerPolicy(cfg, policy)
		if out.Nodes[0].RetryMaxAttempts != 5 {
			t.Fatalf("existing retry should be preserved: %d", out.Nodes[0].RetryMaxAttempts)
		}
	})
}

func TestFinalizeGraphFailurePolicy(t *testing.T) {
	t.Run("no_policy", func(t *testing.T) {
		cfg := GraphBuildConfig{
			Nodes: []NodeDef{{ID: "n1", Type: "agent", FailureAction: FailureDefaultRetryThenBlock}},
		}
		out := FinalizeGraphFailurePolicy(cfg, nil, nil)
		if out.Nodes[0].FailureAction != FailureDefaultRetryThenBlock {
			t.Fatalf("no policy should not change action")
		}
	})

	t.Run("with_skip_policy", func(t *testing.T) {
		cfg := GraphBuildConfig{
			Nodes: []NodeDef{{ID: "n1", Type: "agent", FailureAction: FailureDefaultSkip}},
		}
		out := FinalizeGraphFailurePolicy(cfg, nil, nil)
		if out.Nodes[0].Type != "function" {
			t.Fatalf("skip node should be converted to function: type=%q", out.Nodes[0].Type)
		}
		found := false
		for _, sf := range out.StateFields {
			if sf.Name == SkippedNodesStateKey {
				found = true
			}
		}
		if !found {
			t.Fatal("expected _skipped_nodes state field after finalize")
		}
	})
}

func TestParallelBranchNodeIDs_noFinishPoint(t *testing.T) {
	cfg := GraphBuildConfig{
		Edges: []EdgeDef{{From: "a", To: "b"}},
	}
	branches := parallelBranchNodeIDs(cfg, nil)
	if branches != nil {
		t.Fatalf("expected nil without finish point, got %v", branches)
	}
}

func TestParallelBranchNodeIDs_singleFeeder(t *testing.T) {
	cfg := GraphBuildConfig{
		FinishPoint: "end",
		Edges:       []EdgeDef{{From: "a", To: "end"}},
	}
	branches := parallelBranchNodeIDs(cfg, nil)
	if branches != nil {
		t.Fatalf("expected nil with single feeder, got %v", branches)
	}
}

func TestParallelBranchNodeIDs_transferEdgesIgnored(t *testing.T) {
	cfg := GraphBuildConfig{
		FinishPoint: "end",
		Edges: []EdgeDef{
			{From: "a", To: "end"},
			{From: "b", To: "end", Kind: "transfer"},
		},
	}
	branches := parallelBranchNodeIDs(cfg, nil)
	if branches != nil {
		t.Fatalf("transfer edges should not count as feeders, got %v", branches)
	}
}

func TestNormalizeFailureDefault(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"", FailureDefaultRetryThenBlock},
		{"skip", FailureDefaultSkip},
		{"SKIP", FailureDefaultSkip},
		{"fail_fast", FailureDefaultFailFast},
		{"FAIL_FAST", FailureDefaultFailFast},
		{"retry_then_block", FailureDefaultRetryThenBlock},
		{"unknown", FailureDefaultRetryThenBlock},
		{"  skip  ", FailureDefaultSkip},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := normalizeFailureDefault(tt.input); got != tt.want {
				t.Errorf("normalizeFailureDefault(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
