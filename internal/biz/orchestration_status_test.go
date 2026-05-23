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

func TestOrchestrationStatusStore_MemberFlow(t *testing.T) {
	reg := testRegistry()
	store := NewOrchestrationStatusStore(reg)

	start := contract.NewEnvelope(contract.EnvelopeTypeMemberMessageStart, "worker-a", "sess-1")
	start.ToolCall = &contract.EnvelopeToolCall{AgentKey: "worker-a", AgentID: "a1"}
	changed := store.ApplyEnvelope(start, reg)
	if len(changed) != 1 || changed[0].Status != AgentNodeStatusThinking {
		t.Fatalf("start: got %+v", changed)
	}

	tool := contract.NewEnvelope(contract.EnvelopeTypeToolCall, "worker-a", "sess-1")
	tool.ToolCall = &contract.EnvelopeToolCall{
		AgentKey: "worker-a",
		Name:     "read_file",
		Status:   "running",
	}
	changed = store.ApplyEnvelope(tool, reg)
	if len(changed) != 1 || changed[0].Status != AgentNodeStatusToolRunning {
		t.Fatalf("tool: got %+v", changed)
	}

	done := contract.NewEnvelope(contract.EnvelopeTypeMemberMessageDone, "worker-a", "sess-1")
	done.ToolCall = &contract.EnvelopeToolCall{AgentKey: "worker-a"}
	done.Content = &contract.EnvelopeContent{Text: "finished output"}
	changed = store.ApplyEnvelope(done, reg)
	if len(changed) != 1 {
		t.Fatalf("done: expected change")
	}
	st := store.Nodes["member-1"]
	if st.Status != AgentNodeStatusSuccess || st.Phase != WorkPhaseDelivered {
		t.Fatalf("final: status=%s phase=%s", st.Status, st.Phase)
	}
	if st.OutputPreview != "finished output" {
		t.Fatalf("output preview: %q", st.OutputPreview)
	}
}

func TestOrchestrationStatusStore_TerminalNotOverwritten(t *testing.T) {
	reg := testRegistry()
	store := NewOrchestrationStatusStore(reg)

	done := contract.NewEnvelope(contract.EnvelopeTypeMemberMessageDone, "worker-a", "sess-1")
	done.ToolCall = &contract.EnvelopeToolCall{AgentKey: "worker-a"}
	store.ApplyEnvelope(done, reg)

	start := contract.NewEnvelope(contract.EnvelopeTypeMemberMessageStart, "worker-a", "sess-1")
	start.ToolCall = &contract.EnvelopeToolCall{AgentKey: "worker-a"}
	changed := store.ApplyEnvelope(start, reg)
	if len(changed) != 0 {
		t.Fatalf("terminal should not be overwritten by start")
	}
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

func TestOrchestrationStatusStore_Transfer(t *testing.T) {
	reg := testRegistry()
	store := NewOrchestrationStatusStore(reg)

	tr := contract.NewEnvelope(contract.EnvelopeTypeTransfer, "team", "sess-1")
	tr.Transfer = &contract.EnvelopeTransfer{FromAgent: "worker-a", ToAgent: "worker-b"}
	changed := store.ApplyEnvelope(tr, reg)
	if len(changed) < 1 {
		t.Fatalf("transfer: expected changes, got %d", len(changed))
	}
	if store.Nodes["member-2"].Status != AgentNodeStatusRunning {
		t.Fatalf("target status: %s", store.Nodes["member-2"].Status)
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

func TestActivityHistoryProjection(t *testing.T) {
	reg := testRegistry()
	store := NewOrchestrationStatusStore(reg)

	start := contract.NewEnvelope(contract.EnvelopeTypeMemberMessageStart, "worker-a", "sess-1")
	start.ToolCall = &contract.EnvelopeToolCall{AgentKey: "worker-a", AgentID: "a1"}
	store.ApplyEnvelope(start, reg)

	for i := 0; i < 10; i++ {
		tool := contract.NewEnvelope(contract.EnvelopeTypeToolCall, "worker-a", "sess-1")
		tool.ToolCall = &contract.EnvelopeToolCall{
			AgentKey:     "worker-a",
			Name:         "read_file",
			DisplayLabel: "read_file",
			Status:       "running",
			StartedAt:    "2026-05-23T00:00:00Z",
		}
		store.ApplyEnvelope(tool, reg)

		result := contract.NewEnvelope(contract.EnvelopeTypeToolResult, "worker-a", "sess-1")
		result.ToolCall = &contract.EnvelopeToolCall{
			AgentKey:   "worker-a",
			Name:       "read_file",
			Status:     "success",
			FinishedAt: "2026-05-23T00:00:01Z",
		}
		store.ApplyEnvelope(result, reg)
	}

	st := store.Nodes["member-1"]
	if st == nil {
		t.Fatal("missing member-1 state")
	}
	if st.CurrentActivity == nil {
		t.Fatal("expected current activity")
	}
	if st.CurrentActivity.Status != "success" {
		t.Fatalf("current status=%q want success", st.CurrentActivity.Status)
	}
	if len(st.ActivityHistory) < 10 {
		t.Fatalf("activity history len=%d want >= 10", len(st.ActivityHistory))
	}
	last := st.ActivityHistory[len(st.ActivityHistory)-1]
	if last.FinishedAt == "" {
		t.Fatal("last history entry should have finished_at")
	}
}
