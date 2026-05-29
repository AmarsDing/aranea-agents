package kanban

import (
	"testing"
)

func TestNewToolset_nilBridge(t *testing.T) {
	t.Parallel()

	tools := NewToolset(nil)
	if tools != nil {
		t.Fatalf("expected nil for nil bridge, got %v", tools)
	}
}

func TestNewToolset_withBridge(t *testing.T) {
	t.Parallel()

	b := &stubBridge{}
	tools := NewToolset(b)
	if tools == nil {
		t.Fatal("expected non-nil tools for non-nil bridge")
	}

	expectedCount := 9
	if len(tools) != expectedCount {
		t.Fatalf("expected %d tools, got %d", expectedCount, len(tools))
	}

	expectedNames := []string{
		"kanban_show",
		"kanban_list",
		"kanban_complete",
		"kanban_block",
		"kanban_unblock",
		"kanban_heartbeat",
		"kanban_comment",
		"kanban_create",
		"kanban_link",
	}
	for i, name := range expectedNames {
		decl := tools[i].Declaration()
		if decl.Name != name {
			t.Fatalf("tool[%d]: expected name %q, got %q", i, name, decl.Name)
		}
	}
}

func TestNewToolset_toolDeclarationsHaveDescriptions(t *testing.T) {
	t.Parallel()

	b := &stubBridge{}
	tools := NewToolset(b)
	for i, tool := range tools {
		decl := tool.Declaration()
		if decl.Description == "" {
			t.Fatalf("tool[%d] %q has empty description", i, decl.Name)
		}
	}
}

func TestEnabled_noEnvVars(t *testing.T) {
	t.Setenv("ARANEA_TASK_ID", "")
	t.Setenv("ARANEA_KANBAN_TOOLS", "")
	if Enabled() {
		t.Fatal("expected Enabled()=false when no env vars set")
	}
}

func TestEnabled_withTaskID(t *testing.T) {
	t.Setenv("ARANEA_TASK_ID", "task-1")
	t.Setenv("ARANEA_KANBAN_TOOLS", "")
	if !Enabled() {
		t.Fatal("expected Enabled()=true when ARANEA_TASK_ID is set")
	}
}

func TestEnabled_withKanbanTools1(t *testing.T) {
	t.Setenv("ARANEA_TASK_ID", "")
	t.Setenv("ARANEA_KANBAN_TOOLS", "1")
	if !Enabled() {
		t.Fatal("expected Enabled()=true when ARANEA_KANBAN_TOOLS=1")
	}
}

func TestEnabled_kanbanToolsCaseInsensitive(t *testing.T) {
	t.Setenv("ARANEA_TASK_ID", "")
	t.Setenv("ARANEA_KANBAN_TOOLS", "1")
	if !Enabled() {
		t.Fatal("expected Enabled()=true for ARANEA_KANBAN_TOOLS=1 (case insensitive)")
	}
}

func TestEnabled_kanbanToolsOtherValue(t *testing.T) {
	t.Setenv("ARANEA_TASK_ID", "")
	t.Setenv("ARANEA_KANBAN_TOOLS", "yes")
	if Enabled() {
		t.Fatal("expected Enabled()=false when ARANEA_KANBAN_TOOLS is not '1'")
	}
}

func TestEnabled_whitespaceTaskID(t *testing.T) {
	t.Setenv("ARANEA_TASK_ID", "   ")
	t.Setenv("ARANEA_KANBAN_TOOLS", "")
	if Enabled() {
		t.Fatal("expected Enabled()=false when ARANEA_TASK_ID is only whitespace")
	}
}
