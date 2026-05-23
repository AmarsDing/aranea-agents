package biz

import (
	"testing"
	"time"
)

func TestAppendVersionHistoryIncrementsVersion(t *testing.T) {
	previous := &GraphDefinition{
		ID:   "g1",
		Name: "demo",
		Metadata: map[string]any{
			GraphMetadataVersionKey: 2,
		},
		UpdatedAt: time.Now(),
	}
	next := &GraphDefinition{ID: "g1", Name: "demo-v3"}
	appendVersionHistory(next, previous)
	if next.Version != 3 {
		t.Fatalf("version=%d want 3", next.Version)
	}
	history := ListGraphVersionEntries(next)
	if len(history) != 1 || history[0].Version != 2 {
		t.Fatalf("history=%+v", history)
	}
}

func TestFindGraphVersionSnapshot(t *testing.T) {
	snap := &GraphDefinition{ID: "g1", Name: "old", Version: 1}
	def := &GraphDefinition{
		ID: "g1",
		Metadata: map[string]any{
			GraphMetadataVersionHistoryKey: []GraphVersionEntry{{
				Version: 1, Snapshot: snap,
			}},
		},
	}
	found := FindGraphVersionSnapshot(def, 1)
	if found == nil || found.Name != "old" {
		t.Fatalf("found=%+v", found)
	}
}
