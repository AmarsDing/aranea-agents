package biz

import "testing"

func TestFilesForMode_Minimized(t *testing.T) {
	files := []AgentPromptFile{
		{Name: "AGENTS_CORE.md"},
		{Name: "IDENTITY.md"},
		{Name: "RULE.md"},
		{Name: "AGENTS_TASK.md"},
	}
	got := FilesForMode(files, "minimized")
	if len(got) != 2 {
		t.Fatalf("minimized: got %d files", len(got))
	}
	if got[0].Name != "AGENTS_CORE.md" || got[1].Name != "RULE.md" {
		t.Fatalf("minimized allowlist: %#v", got)
	}
}

func TestFilesForMode_Task(t *testing.T) {
	files := []AgentPromptFile{
		{Name: "AGENTS_CORE.md"},
		{Name: "IDENTITY.md"},
		{Name: "EXTRA.md"},
	}
	got := FilesForMode(files, "task")
	if len(got) != 2 {
		t.Fatalf("task: got %d files", len(got))
	}
}
