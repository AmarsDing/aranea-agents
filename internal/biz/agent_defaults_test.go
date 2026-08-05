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

// FR-12/P2: L2 召回默认开（评审 V7）。回归守卫：新 agent 的 standard 记忆
// 档位必须包含 L2 召回，否则回到「五个层里两个半在干活」的默认配置。
func TestDefaultAgentRuntimeSettings_L2RecallEnabled(t *testing.T) {
	s := DefaultAgentRuntimeSettings()
	if !s.L2RecallEnabled {
		t.Error("L2RecallEnabled should default to true (FR-12/P2)")
	}
	p := ResolveMemoryRuntimePolicy(&s)
	if !p.RecallL2 {
		t.Error("resolved policy should recall L2 by default")
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
