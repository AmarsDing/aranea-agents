package graph

import (
	"testing"
	"time"

	trpcgraph "trpc.group/trpc-go/trpc-agent-go/graph"
)

func TestExecutionSummaryTracker(t *testing.T) {
	tr := NewExecutionSummaryTracker("exec-1", "g-1")
	tr.RecordNodeComplete(trpcgraph.NodeExecutionMetadata{
		NodeID: "n1", NodeType: trpcgraph.NodeTypeLLM, Duration: 50 * time.Millisecond,
	})
	summary := tr.Snapshot(trpcgraph.CompletionMetadata{TotalSteps: 1, TotalDuration: 100 * time.Millisecond})
	if summary.ExecutionID != "exec-1" || len(summary.Nodes) != 1 {
		t.Fatalf("summary=%+v", summary)
	}
}

func TestExecutionSummaryTracker_MultipleNodes(t *testing.T) {
	tr := NewExecutionSummaryTracker("exec-multi", "g-multi")
	tr.RecordNodeComplete(trpcgraph.NodeExecutionMetadata{
		NodeID: "n1", NodeType: trpcgraph.NodeTypeLLM, Duration: 30 * time.Millisecond, StepNumber: 1,
	})
	tr.RecordNodeComplete(trpcgraph.NodeExecutionMetadata{
		NodeID: "n2", NodeType: trpcgraph.NodeTypeFunction, Duration: 60 * time.Millisecond, StepNumber: 2,
	})
	tr.RecordNodeComplete(trpcgraph.NodeExecutionMetadata{
		NodeID: "n3", NodeType: trpcgraph.NodeTypeLLM, Duration: 90 * time.Millisecond, StepNumber: 3,
	})
	summary := tr.Snapshot(trpcgraph.CompletionMetadata{TotalSteps: 3, TotalDuration: 180 * time.Millisecond})
	if summary.ExecutionID != "exec-multi" || summary.GraphID != "g-multi" {
		t.Fatalf("ids: executionID=%s graphID=%s", summary.ExecutionID, summary.GraphID)
	}
	if len(summary.Nodes) != 3 {
		t.Fatalf("expected 3 nodes, got %d", len(summary.Nodes))
	}
	if summary.Nodes[0].NodeID != "n1" || summary.Nodes[1].NodeID != "n2" || summary.Nodes[2].NodeID != "n3" {
		t.Fatalf("node order: %s %s %s", summary.Nodes[0].NodeID, summary.Nodes[1].NodeID, summary.Nodes[2].NodeID)
	}
	if summary.Nodes[0].DurationMS != 30 || summary.Nodes[1].DurationMS != 60 || summary.Nodes[2].DurationMS != 90 {
		t.Fatalf("durations: %d %d %d", summary.Nodes[0].DurationMS, summary.Nodes[1].DurationMS, summary.Nodes[2].DurationMS)
	}
	if summary.TotalSteps != 3 || summary.DurationMS != 180 {
		t.Fatalf("completion: totalSteps=%d durationMS=%d", summary.TotalSteps, summary.DurationMS)
	}
}

func TestExecutionSummaryTracker_NodeStatusSuccess(t *testing.T) {
	tr := NewExecutionSummaryTracker("exec-ok", "g-ok")
	tr.RecordNodeComplete(trpcgraph.NodeExecutionMetadata{
		NodeID: "n1", NodeType: trpcgraph.NodeTypeLLM, Duration: 50 * time.Millisecond,
	})
	summary := tr.Snapshot(trpcgraph.CompletionMetadata{TotalSteps: 1, TotalDuration: 50 * time.Millisecond})
	if len(summary.Nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(summary.Nodes))
	}
	if summary.Nodes[0].Status != NodeExecStatusSuccess {
		t.Fatalf("expected status %q, got %q", NodeExecStatusSuccess, summary.Nodes[0].Status)
	}
	if summary.Nodes[0].Error != "" {
		t.Fatalf("expected empty error, got %q", summary.Nodes[0].Error)
	}
}

func TestExecutionSummaryTracker_NodeStatusError(t *testing.T) {
	tr := NewExecutionSummaryTracker("exec-err", "g-err")
	tr.RecordNodeComplete(trpcgraph.NodeExecutionMetadata{
		NodeID: "n1", NodeType: trpcgraph.NodeTypeFunction, Duration: 20 * time.Millisecond, Error: "boom",
	})
	summary := tr.Snapshot(trpcgraph.CompletionMetadata{TotalSteps: 1, TotalDuration: 20 * time.Millisecond})
	if len(summary.Nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(summary.Nodes))
	}
	if summary.Nodes[0].Status != NodeExecStatusError {
		t.Fatalf("expected status %q, got %q", NodeExecStatusError, summary.Nodes[0].Status)
	}
	if summary.Nodes[0].Error != "boom" {
		t.Fatalf("expected error %q, got %q", "boom", summary.Nodes[0].Error)
	}
}

func TestExecutionSummaryTracker_EmptySnapshot(t *testing.T) {
	tr := NewExecutionSummaryTracker("exec-empty", "g-empty")
	summary := tr.Snapshot(trpcgraph.CompletionMetadata{})
	if summary.ExecutionID != "exec-empty" || summary.GraphID != "g-empty" {
		t.Fatalf("ids: executionID=%s graphID=%s", summary.ExecutionID, summary.GraphID)
	}
	if len(summary.Nodes) != 0 {
		t.Fatalf("expected 0 nodes, got %d", len(summary.Nodes))
	}
	if summary.TotalSteps != 0 || summary.DurationMS != 0 || summary.FinalStateKeys != 0 {
		t.Fatalf("expected zero completion fields: totalSteps=%d durationMS=%d finalStateKeys=%d",
			summary.TotalSteps, summary.DurationMS, summary.FinalStateKeys)
	}
}

func TestExecutionSummaryTracker_SnapshotCompletionMeta(t *testing.T) {
	tr := NewExecutionSummaryTracker("exec-meta", "g-meta")
	tr.RecordNodeComplete(trpcgraph.NodeExecutionMetadata{
		NodeID: "n1", NodeType: trpcgraph.NodeTypeLLM, Duration: 100 * time.Millisecond, StepNumber: 1,
	})
	summary := tr.Snapshot(trpcgraph.CompletionMetadata{
		TotalSteps:     5,
		TotalDuration:  500 * time.Millisecond,
		FinalStateKeys: 3,
	})
	if summary.TotalSteps != 5 {
		t.Fatalf("expected TotalSteps=5, got %d", summary.TotalSteps)
	}
	if summary.DurationMS != 500 {
		t.Fatalf("expected DurationMS=500, got %d", summary.DurationMS)
	}
	if summary.FinalStateKeys != 3 {
		t.Fatalf("expected FinalStateKeys=3, got %d", summary.FinalStateKeys)
	}
	if len(summary.Nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(summary.Nodes))
	}
	if summary.Nodes[0].StepNumber != 1 {
		t.Fatalf("expected StepNumber=1, got %d", summary.Nodes[0].StepNumber)
	}
}
