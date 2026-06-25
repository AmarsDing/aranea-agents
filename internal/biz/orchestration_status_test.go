package biz

import (
	"testing"

	"aranea-agents/internal/event/contract"
)

func testRegistry() OrchestrationRegistry {
	return NewOrchestrationRegistry([]OrchestrationNodeRegistryEntry{
		{NodeID: "member-1", AgentID: "a1", AgentKey: "worker-a", AgentName: "Worker A", Role: "worker"},
		{NodeID: "member-2", AgentID: "a2", AgentKey: "worker-b", AgentName: "Worker B", Role: "worker"},
	})
}

func TestOrchestrationStatusStore_GraphTaskReviewRequired(t *testing.T) {
	reg := NewOrchestrationRegistry([]OrchestrationNodeRegistryEntry{
		{NodeID: "member-1", AgentID: "a1", AgentKey: "worker-a"},
	})
	store := NewOrchestrationStatusStore(reg)
	env := contract.NewEnvelope(contract.EnvelopeTypeGraphTaskStatus, "graph-task", "sess-1")
	env.Metadata = map[string]any{
		"node_id":     "member-1",
		"task_status": "review_required",
		"summary":     "needs review",
	}
	changed := store.ApplyEnvelope(env, reg)
	if len(changed) != 1 {
		t.Fatalf("changed=%d want 1", len(changed))
	}
	if changed[0].Status != AgentNodeStatusWaitingReview {
		t.Fatalf("status=%q", changed[0].Status)
	}
	if changed[0].DisplayStatus != DisplayStatusSuspended {
		t.Fatalf("display=%q", changed[0].DisplayStatus)
	}
}

func TestOrchestrationStatusStore_GraphNodeSkipped(t *testing.T) {
	reg := NewOrchestrationRegistry([]OrchestrationNodeRegistryEntry{
		{NodeID: "member-2", AgentID: "a2", AgentKey: "worker-b"},
	})
	store := NewOrchestrationStatusStore(reg)
	env := contract.NewEnvelope(contract.EnvelopeTypeGraphNodeEnd, "graph", "sess-1")
	env.Metadata = map[string]any{"node_id": "member-2", "skipped": true}
	changed := store.ApplyEnvelope(env, reg)
	if len(changed) != 1 || changed[0].Status != AgentNodeStatusSkipped {
		t.Fatalf("got %+v", changed)
	}
}

func TestApplySkipNodeSemantics(t *testing.T) {
	cfg := GraphBuildConfig{
		Nodes: []NodeDef{{ID: "member-1", Type: "agent", FailureAction: FailureDefaultSkip}},
	}
	out := ApplySkipNodeSemantics(cfg)
	if out.Nodes[0].Type != "function" || out.Nodes[0].FuncRef != SkipNodeFuncRef {
		t.Fatalf("node=%+v", out.Nodes[0])
	}
	if len(out.StateFields) != 1 || out.StateFields[0].Name != SkippedNodesStateKey {
		t.Fatalf("state=%+v", out.StateFields)
	}
}

func TestAggregateDisplayStatus(t *testing.T) {
	if AggregateDisplayStatus(AgentNodeStatusToolRunning) != DisplayStatusActive {
		t.Fatal("tool_running should be active")
	}
	if AggregateDisplayStatus(AgentNodeStatusWaitingInput) != DisplayStatusSuspended {
		t.Fatal("waiting_input should be suspended")
	}
}

func TestOrchestrationStatusStore_FailedThenSkipped(t *testing.T) {
	reg := NewOrchestrationRegistry([]OrchestrationNodeRegistryEntry{
		{NodeID: "member-1", AgentID: "a1", AgentKey: "worker-a"},
	})
	store := NewOrchestrationStatusStore(reg)

	errEnv := contract.NewEnvelope(contract.EnvelopeTypeGraphNodeError, "graph", "sess-1")
	errEnv.Metadata = map[string]any{"node_id": "member-1"}
	errEnv.Error = &contract.EnvelopeError{Message: "boom"}
	store.ApplyEnvelope(errEnv, reg)
	if store.Nodes["member-1"].Status != AgentNodeStatusFailed {
		t.Fatalf("expected failed, got %s", store.Nodes["member-1"].Status)
	}

	skipEnv := contract.NewEnvelope(contract.EnvelopeTypeGraphNodeEnd, "graph", "sess-1")
	skipEnv.Metadata = map[string]any{"node_id": "member-1", "skipped": true}
	changed := store.ApplyEnvelope(skipEnv, reg)
	if len(changed) != 1 || changed[0].Status != AgentNodeStatusSkipped {
		t.Fatalf("failed→skipped override: got %+v", changed)
	}
}

func TestOrchestrationStatusStore_GraphTaskStatusMappings(t *testing.T) {
	reg := NewOrchestrationRegistry([]OrchestrationNodeRegistryEntry{
		{NodeID: "member-1", AgentID: "a1", AgentKey: "worker-a"},
	})
	cases := []struct {
		taskStatus string
		want       AgentNodeStatus
	}{
		{"pending", AgentNodeStatusQueued},
		{"claimed", AgentNodeStatusRunning},
		{"complete", AgentNodeStatusSuccess},
		{"failed", AgentNodeStatusFailed},
		{"crashed", AgentNodeStatusFailed},
		{"timed_out", AgentNodeStatusTimedOut},
		{"cancelled", AgentNodeStatusCancelled},
	}
	for _, tc := range cases {
		store := NewOrchestrationStatusStore(reg)
		env := contract.NewEnvelope(contract.EnvelopeTypeGraphTaskStatus, "graph-task", "sess-1")
		env.Metadata = map[string]any{"node_id": "member-1", "task_status": tc.taskStatus}
		changed := store.ApplyEnvelope(env, reg)
		if len(changed) != 1 || changed[0].Status != tc.want {
			t.Fatalf("%s: got status=%s want=%s", tc.taskStatus, changed[0].Status, tc.want)
		}
	}
}

func TestOrchestrationStatusStore_GraphTaskReviewRejected(t *testing.T) {
	reg := NewOrchestrationRegistry([]OrchestrationNodeRegistryEntry{
		{NodeID: "member-1", AgentID: "a1", AgentKey: "worker-a"},
	})
	store := NewOrchestrationStatusStore(reg)
	env := contract.NewEnvelope(contract.EnvelopeTypeGraphTaskStatus, "graph-task", "sess-1")
	env.Metadata = map[string]any{
		"node_id":         "member-1",
		"task_status":     "claimed",
		"review_rejected": true,
		"review_comment":  "needs rework",
	}
	changed := store.ApplyEnvelope(env, reg)
	if len(changed) != 1 {
		t.Fatalf("changed=%d want 1", len(changed))
	}
	st := changed[0]
	if st.Status != AgentNodeStatusRunning {
		t.Fatalf("status=%q want running", st.Status)
	}
	if st.Phase != WorkPhaseDoing {
		t.Fatalf("phase=%q want doing after rejection", st.Phase)
	}
	if st.ErrorMessage != "needs rework" {
		t.Fatalf("error=%q", st.ErrorMessage)
	}
}

// Phase 1c-5: TestActivityHistoryProjection removed — tested deleted
// EnvelopeType MemberMessageStart/ToolCall/ToolResult behavior in ApplyEnvelope.
