package memory

import (
	"testing"

	"aranea-agents/internal/data/sessionmemory"
)

func TestPreparePersonUpsert_conflict(t *testing.T) {
	existing := sessionmemory.EntitySnapshot{ID: "e1", Name: "Alice", Confidence: 0.8}
	prepared, conflict := preparePersonUpsert(existing, "Bob", "My name is Bob")
	if !conflict {
		t.Fatal("expected conflict when name changes")
	}
	if prepared.MetadataJSON == "" {
		t.Fatal("expected metadata")
	}
	_ = prepared
}

func TestPreparePersonUpsert_noConflictSameName(t *testing.T) {
	existing := sessionmemory.EntitySnapshot{ID: "e1", Name: "Alice", Confidence: 0.8}
	_, conflict := preparePersonUpsert(existing, "Alice", "Alice here")
	if conflict {
		t.Fatal("unexpected conflict for same name")
	}
}
