package biz

import (
	"encoding/json"
	"testing"
)

func TestDefaultAgentRuntimeSettings_L1HistoryEnabled(t *testing.T) {
	s := DefaultAgentRuntimeSettings()
	if s.L1HistoryEnabled {
		t.Error("L1HistoryEnabled should default to false")
	}
}

func TestDefaultToolsDenyFrameworkMemory(t *testing.T) {
	// Verify the constant is valid JSON
	var list []string
	if err := json.Unmarshal([]byte(DefaultToolsDenyFrameworkMemory), &list); err != nil {
		t.Fatalf("DefaultToolsDenyFrameworkMemory is not valid JSON: %v", err)
	}
	// Verify it contains exactly the 5 framework memory tools
	expected := []string{"memory_add", "memory_update", "memory_delete", "memory_search", "memory_load"}
	if len(list) != len(expected) {
		t.Errorf("got %d items, want %d", len(list), len(expected))
	}
	for _, e := range expected {
		found := false
		for _, v := range list {
			if v == e {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing %q in DefaultToolsDenyFrameworkMemory", e)
		}
	}
}

func TestDefaultAgentRuntimeSettings_ToolsDenyFrameworkMemory(t *testing.T) {
	s := DefaultAgentRuntimeSettings()
	if s.ToolsDenyJSON != DefaultToolsDenyFrameworkMemory {
		t.Errorf("ToolsDenyJSON = %q, want %q", s.ToolsDenyJSON, DefaultToolsDenyFrameworkMemory)
	}
	// Verify it denies framework memory tools
	var list []string
	json.Unmarshal([]byte(s.ToolsDenyJSON), &list)
	if len(list) != 5 {
		t.Errorf("expected 5 denied tools, got %d", len(list))
	}
}
