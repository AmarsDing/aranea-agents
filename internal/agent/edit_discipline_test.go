package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"aranea-agents/internal/workspace"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

func TestEditDiscipline_RejectsExistingFile(t *testing.T) {
	base := t.TempDir()
	t.Setenv("ARANEA_WORKSPACE_ROOT", base)
	ag := sandboxAgent("coding", "")
	root := filepath.Join(base, "workspace", workspace.DefaultWorkspaceID, "coder")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	hook := newEditDisciplineBeforeHook(ag, TRPCBuilderDeps{})
	res, err := hook.HandleBeforeTool(context.Background(), &trpctool.BeforeToolArgs{
		ToolName:  "save_file",
		Arguments: []byte(`{"file_name":"main.go","contents":"rewritten"}`),
	})
	if err != nil {
		t.Fatalf("hook err: %v", err)
	}
	msg, _ := res.CustomResult.(string)
	if !strings.Contains(msg, "diff_edit") {
		t.Fatalf("CustomResult = %q, want diff_edit hint", res.CustomResult)
	}
}

func TestEditDiscipline_AllowsNewFile(t *testing.T) {
	base := t.TempDir()
	t.Setenv("ARANEA_WORKSPACE_ROOT", base)
	ag := sandboxAgent("coding", "")
	root := filepath.Join(base, "workspace", workspace.DefaultWorkspaceID, "coder")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	hook := newEditDisciplineBeforeHook(ag, TRPCBuilderDeps{})
	res, err := hook.HandleBeforeTool(context.Background(), &trpctool.BeforeToolArgs{
		ToolName:  "file_save_file",
		Arguments: []byte(`{"file_name":"new.go","contents":"package main"}`),
	})
	if err != nil {
		t.Fatalf("hook err: %v", err)
	}
	if res != nil && res.CustomResult != nil {
		t.Fatalf("new file must pass, got CustomResult=%v", res.CustomResult)
	}
}

func TestEditDiscipline_SkipsResearchProfile(t *testing.T) {
	base := t.TempDir()
	t.Setenv("ARANEA_WORKSPACE_ROOT", base)
	ag := sandboxAgent("research", `["save_file"]`)
	root := filepath.Join(base, "workspace", workspace.DefaultWorkspaceID, "coder")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "notes.md"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	hook := newEditDisciplineBeforeHook(ag, TRPCBuilderDeps{})
	res, err := hook.HandleBeforeTool(context.Background(), &trpctool.BeforeToolArgs{
		ToolName:  "save_file",
		Arguments: []byte(`{"file_name":"notes.md","contents":"new"}`),
	})
	if err != nil {
		t.Fatalf("hook err: %v", err)
	}
	if res != nil && res.CustomResult != nil {
		t.Fatalf("research profile must not enforce coding edit discipline, got %v", res.CustomResult)
	}
}

func TestEditDisciplineHookPriority(t *testing.T) {
	t.Parallel()
	hook := newEditDisciplineBeforeHook(sandboxAgent("coding", ""), TRPCBuilderDeps{})
	if hook.Priority() != editDisciplineHookPriority {
		t.Fatalf("priority = %d, want %d", hook.Priority(), editDisciplineHookPriority)
	}
	if editDisciplineHookPriority <= workspaceSandboxHookPriority || editDisciplineHookPriority >= 10 {
		t.Fatal("edit discipline must sit between sandbox (8) and confirm (10)")
	}
}
