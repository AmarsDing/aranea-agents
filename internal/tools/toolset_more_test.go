package tools

import (
	"context"
	"testing"
)

func TestAssemble_emptyEnabledTools(t *testing.T) {
	out, err := Assemble(context.Background(), AssemblyConfig{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out == nil {
		t.Fatal("expected non-nil result")
	}
	if len(out.Tools) != 0 {
		t.Fatalf("expected 0 tools, got %d", len(out.Tools))
	}
	if len(out.ToolSets) != 0 {
		t.Fatalf("expected 0 toolsets, got %d", len(out.ToolSets))
	}
}

func TestAssemble_todoEnabled(t *testing.T) {
	out, err := Assemble(context.Background(), AssemblyConfig{
		EnabledTools: []string{"todo"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	found := false
	for _, tool := range out.Tools {
		if d := tool.Declaration(); d != nil && d.Name == "todo_write" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected todo_write tool")
	}
}

func TestAssemble_duckduckgoEnabled(t *testing.T) {
	out, err := Assemble(context.Background(), AssemblyConfig{
		EnabledTools: []string{"duckduckgo"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	found := false
	for _, tool := range out.Tools {
		if d := tool.Declaration(); d != nil && d.Name == "duckduckgo_search" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected duckduckgo_search tool")
	}
}

func TestAssemble_awaitUserReplyEnabled(t *testing.T) {
	out, err := Assemble(context.Background(), AssemblyConfig{
		EnabledTools: []string{"await_user_reply"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	found := false
	for _, tool := range out.Tools {
		if d := tool.Declaration(); d != nil && d.Name == "await_user_reply" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected await_user_reply tool")
	}
}

func TestAssemble_memoryEnabled(t *testing.T) {
	out, err := Assemble(context.Background(), AssemblyConfig{
		Session: SessionConfig{MemoryEnabled: true},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	memNames := map[string]bool{
		"memory_add": true, "memory_update": true, "memory_load": true,
		"memory_search": true, "memory_delete": true,
	}
	found := 0
	for _, tool := range out.Tools {
		if d := tool.Declaration(); d != nil && memNames[d.Name] {
			found++
		}
	}
	if found < 5 {
		t.Fatalf("expected 5 memory tools, found %d", found)
	}
}

func TestAssemble_customTools(t *testing.T) {
	custom := &mockToolForAlias{decl: &Declaration{Name: "my_custom_tool"}}
	out, err := Assemble(context.Background(), AssemblyConfig{
		Session: SessionConfig{CustomTools: []Tool{custom}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	found := false
	for _, tool := range out.Tools {
		if d := tool.Declaration(); d != nil && d.Name == "my_custom_tool" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected my_custom_tool in assembled tools")
	}
}

// TestAssemble_duplicateCustomToolNames guards the earlier-wins dedup: two
// flat tools with the same declaration name must collapse to the first one
// instead of reaching the model twice (most LLM APIs reject duplicate names).
func TestAssemble_duplicateCustomToolNames(t *testing.T) {
	first := &mockToolForAlias{decl: &Declaration{Name: "dup_tool", Description: "first"}}
	second := &mockToolForAlias{decl: &Declaration{Name: "dup_tool", Description: "second"}}
	out, err := Assemble(context.Background(), AssemblyConfig{
		Session: SessionConfig{CustomTools: []Tool{first, second}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	count := 0
	for _, tool := range out.Tools {
		if d := tool.Declaration(); d != nil && d.Name == "dup_tool" {
			count++
			if d.Description != "first" {
				t.Fatalf("expected earlier-wins, got description %q", d.Description)
			}
		}
	}
	if count != 1 {
		t.Fatalf("expected exactly 1 dup_tool after dedup, got %d", count)
	}
}

// TestAssemble_deliverableNotMountedFromRegistry guards the deliverable
// placeholder contract: the registry entry must not mount the uncontracted
// ToolSet — team runtime injects contract-aware tools via CustomTools.
func TestAssemble_deliverableNotMountedFromRegistry(t *testing.T) {
	out, err := Assemble(context.Background(), AssemblyConfig{
		EnabledTools: []string{"deliverable"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, ts := range out.ToolSets {
		if ts != nil && ts.Name() == "deliverable" {
			t.Fatal("deliverable ToolSet must not be mounted from the registry (assembled elsewhere)")
		}
	}
}

func TestAssemble_arxivSearchEnabled(t *testing.T) {
	out, err := Assemble(context.Background(), AssemblyConfig{
		EnabledTools: []string{"arxiv_search"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.ToolSets) == 0 {
		t.Fatal("expected at least one toolset for arxiv_search")
	}
}

func TestAssemble_wikipediaEnabled(t *testing.T) {
	out, err := Assemble(context.Background(), AssemblyConfig{
		EnabledTools: []string{"wikipedia"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.ToolSets) == 0 {
		t.Fatal("expected at least one toolset for wikipedia")
	}
}

func TestAssemble_unknownToolIgnored(t *testing.T) {
	out, err := Assemble(context.Background(), AssemblyConfig{
		EnabledTools: []string{"nonexistent_tool_xyz"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Tools) != 0 || len(out.ToolSets) != 0 {
		t.Fatal("unknown tool should be ignored")
	}
}

func TestAssemble_registryToolWithNilFactory(t *testing.T) {
	out, err := Assemble(context.Background(), AssemblyConfig{
		EnabledTools: []string{"read_tool_result"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	found := false
	for _, tool := range out.Tools {
		if d := tool.Declaration(); d != nil {
			found = true
		}
	}
	for _, ts := range out.ToolSets {
		if ts != nil {
			found = true
		}
	}
	_ = found
}

func TestRegistry_containsExpectedTools(t *testing.T) {
	regs := Registry()
	names := map[string]bool{}
	for _, r := range regs {
		names[r.Name] = true
	}
	expected := []string{"file", "hostexec", "httpfetch", "duckduckgo", "todo", "email", "wikipedia", "arxiv_search"}
	for _, name := range expected {
		if !names[name] {
			t.Errorf("expected %q in registry", name)
		}
	}
}

func TestRegistry_uniqueNames(t *testing.T) {
	regs := Registry()
	seen := map[string]bool{}
	for _, r := range regs {
		if seen[r.Name] {
			t.Errorf("duplicate registration name: %q", r.Name)
		}
		seen[r.Name] = true
	}
}
