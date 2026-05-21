package a2a

import (
	"testing"

	trpcgraph "trpc.group/trpc-go/trpc-agent-go/graph"
)

func TestBuildGraphResumeMetadata(t *testing.T) {
	t.Parallel()
	meta := BuildGraphResumeMetadata(GraphResumeInput{
		LineageID:    "ln-1",
		CheckpointID: "ck-1",
		CheckpointNS: "ns-1",
		ResumeMap:    map[string]any{"approve": true},
	})
	if meta == nil {
		t.Fatal("expected metadata")
	}
	if meta[trpcgraph.CfgKeyLineageID] != "ln-1" {
		t.Fatalf("lineage_id mismatch: %#v", meta)
	}
	if meta[trpcgraph.CfgKeyCheckpointID] != "ck-1" {
		t.Fatalf("checkpoint_id mismatch: %#v", meta)
	}
	if _, ok := meta[trpcgraph.CfgKeyResumeMap]; !ok {
		t.Fatalf("expected resume map: %#v", meta)
	}
	if _, ok := meta[MessageMetadataStateDeltaKey]; ok {
		t.Fatal("flattened metadata must not nest state_delta")
	}
}

func TestBuildGraphResumeMetadataEmpty(t *testing.T) {
	t.Parallel()
	if BuildGraphResumeMetadata(GraphResumeInput{}) != nil {
		t.Fatal("expected nil for empty input")
	}
}
