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
