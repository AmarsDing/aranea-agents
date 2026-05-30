package memory

import (
	"testing"
)

func TestDefaultTools_Count(t *testing.T) {
	tools := DefaultTools()
	if len(tools) != 5 {
		t.Fatalf("len(DefaultTools()) = %d, want 5", len(tools))
	}
}

func TestDefaultTools_DeclarationNames(t *testing.T) {
	expectedNames := map[string]bool{
		"memory_add":    true,
		"memory_update": true,
		"memory_load":   true,
		"memory_search": true,
		"memory_delete": true,
	}
	tools := DefaultTools()
	for _, tool := range tools {
		d := tool.Declaration()
		if d == nil {
			t.Fatal("tool has nil Declaration")
		}
		if !expectedNames[d.Name] {
			t.Fatalf("unexpected tool name %q", d.Name)
		}
		delete(expectedNames, d.Name)
	}
	if len(expectedNames) > 0 {
		t.Fatalf("missing tools: %v", expectedNames)
	}
}

func TestDefaultTools_NilCheck(t *testing.T) {
	tools := DefaultTools()
	for i, tool := range tools {
		if tool == nil {
			t.Fatalf("tool[%d] is nil", i)
		}
	}
}
