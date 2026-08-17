package tools

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"aranea-agents/pkg/loggateway"
)

func TestNewAssembledToolHandler_SaveFilePathAlias(t *testing.T) {
	dir := t.TempDir()
	ts, err := Assemble(context.Background(), AssemblyConfig{
		EnabledTools:  []string{"file"},
		FilesystemDir: dir,
		Lg:            loggateway.NewNoop(),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		for _, set := range ts.ToolSets {
			_ = set.Close()
		}
	})
	ApplyDefaultDecorators(ts, loggateway.NewNoop())
	handler := NewAssembledToolHandler(ts.Tools, ts.ToolSets)
	res := handler(context.Background(), ToolCall{
		ID:        "1",
		Name:      "save_file",
		Arguments: []byte(`{"path":"from-handler.txt","content":"ok","overwrite":true}`),
	})
	if !res.Success {
		t.Fatalf("save_file: %s", res.Error)
	}
	body, err := os.ReadFile(filepath.Join(dir, "from-handler.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "ok" {
		t.Fatalf("file body = %q", body)
	}
}

func TestNewAssembledToolHandler_MissingTool(t *testing.T) {
	handler := NewAssembledToolHandler(nil, nil)
	res := handler(context.Background(), ToolCall{ID: "1", Name: "save_file"})
	if res.Success || res.Error == "" {
		t.Fatalf("expected missing-tool error, got %+v", res)
	}
}

func TestBatchExecuteAssembledTools_StripsWorktreeIsolation(t *testing.T) {
	dir := t.TempDir()
	ts, err := Assemble(context.Background(), AssemblyConfig{
		EnabledTools:  []string{"file"},
		FilesystemDir: dir,
		Lg:            loggateway.NewNoop(),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		for _, set := range ts.ToolSets {
			_ = set.Close()
		}
	})
	ApplyDefaultDecorators(ts, loggateway.NewNoop())

	gitRoot, cleanup := initTempGitRepo(t)
	defer cleanup()
	iso, err := NewWorktreeIsolator(gitRoot, nil, loggateway.NewNoop())
	if err != nil {
		t.Fatalf("NewWorktreeIsolator: %v", err)
	}
	exec := NewParallelToolExecutor(nil, loggateway.NewNoop(),
		WithMaxConcurrency(2), WithWorktreeIsolator(iso))

	results := BatchExecuteAssembledTools(context.Background(), exec, ts, []ToolCall{{
		ID:                "1",
		Name:              "save_file",
		Arguments:         []byte(`{"file_name":"assembled.txt","contents":"ok","overwrite":true}`),
		IsolationStrategy: IsolationStrategyWorktree,
	}}, loggateway.NewNoop())
	if len(results) != 1 || !results[0].Success {
		t.Fatalf("save_file: %+v", results)
	}
	body, err := os.ReadFile(filepath.Join(dir, "assembled.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "ok" {
		t.Fatalf("live workspace body = %q", body)
	}
	if _, err := os.Stat(filepath.Join(gitRoot, "assembled.txt")); err == nil {
		t.Fatal("worktree isolator must not receive assembled file writes")
	}
}
