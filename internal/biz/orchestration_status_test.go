package biz

import (
	"testing"
)

func testRegistry() OrchestrationRegistry {
	return NewOrchestrationRegistry([]OrchestrationNodeRegistryEntry{
		{NodeID: "member-1", AgentID: "a1", AgentKey: "worker-a", AgentName: "Worker A", Role: "worker"},
		{NodeID: "member-2", AgentID: "a2", AgentKey: "worker-b", AgentName: "Worker B", Role: "worker"},
	})
}

func TestOrchestrationStatusStore_GraphNodeSkipped(t *testing.T) {
	reg := NewOrchestrationRegistry([]OrchestrationNodeRegistryEntry{
		{NodeID: "member-2", AgentID: "a2", AgentKey: "worker-b"},
	})
	store := NewOrchestrationStatusStore(reg)
	aev := ActivityEvent{
		Event: ActivityEventCompleted,
		Activity: Activity{
			Kind:      ActivityKindGraphStage,
			Stage:     "node_end",
			SessionID: "sess-1",
			Meta:      map[string]any{"node_id": "member-2", "skipped": true},
		},
	}
	changed := store.ApplyActivityEvent(aev, reg)
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

	errAev := ActivityEvent{
		Event: ActivityEventFailed,
		Activity: Activity{
			Kind:      ActivityKindGraphStage,
			Stage:     "node_error",
			SessionID: "sess-1",
			Content:   "boom",
			Meta:      map[string]any{"node_id": "member-1"},
		},
	}
	store.ApplyActivityEvent(errAev, reg)
	if store.Nodes["member-1"].Status != AgentNodeStatusFailed {
		t.Fatalf("expected failed, got %s", store.Nodes["member-1"].Status)
	}

	skipAev := ActivityEvent{
		Event: ActivityEventCompleted,
		Activity: Activity{
			Kind:      ActivityKindGraphStage,
			Stage:     "node_end",
			SessionID: "sess-1",
			Meta:      map[string]any{"node_id": "member-1", "skipped": true},
		},
	}
	changed := store.ApplyActivityEvent(skipAev, reg)
	if len(changed) != 1 || changed[0].Status != AgentNodeStatusSkipped {
		t.Fatalf("failed→skipped override: got %+v", changed)
	}
}

// Phase 1c-5: GraphTaskReviewRequired / GraphTaskStatusMappings / GraphTaskReviewRejected
// tests removed — applyGraphTaskStatus was dead code (no producer for
// EnvelopeTypeGraphTaskStatus) and has been deleted along with the Envelope bridge.
