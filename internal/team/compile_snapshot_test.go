package team

import (
	"testing"
)

func TestDefinitionSnapshotOnRun(t *testing.T) {
	raw := `{"mode":"sequential","members":[{"agent_id":"a1","sort_order":1}]}`
	if snap := BuildCompileSnapshot(Definition{Mode: "sequential", Members: []MemberDef{{AgentID: "a1", SortOrder: 1}}}, raw, nil); !snap.Valid {
		t.Fatalf("expected valid compile from snapshot json")
	}
}
