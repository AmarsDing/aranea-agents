package team

import (
	"testing"

	"aranea-agents/pkg/loggateway"
)

func TestExportStructureSnapshot_sequential(t *testing.T) {
	def := Definition{
		Mode: "sequential",
		Members: []MemberDef{
			{AgentID: "a1", SortOrder: 1, Name: "Worker A"},
			{AgentID: "a2", SortOrder: 2, Name: "Worker B"},
		},
	}
	snap, err := ExportStructureSnapshot("demo", "Demo Team", def, nil, loggateway.NewNoop())
	if err != nil {
		t.Fatal(err)
	}
	if snap.EntryNodeID != "team-demo" {
		t.Fatalf("entry=%q", snap.EntryNodeID)
	}
	if len(snap.Nodes) != 3 {
		t.Fatalf("nodes=%d", len(snap.Nodes))
	}
	if len(snap.Edges) != 2 {
		t.Fatalf("edges=%d", len(snap.Edges))
	}
}
