package deletefile

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"aranea-agents/pkg/loggateway"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

func TestDeleteFileRemovesWorkspaceFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	tool := NewTool(dir, loggateway.NewNoop())
	ct := tool.(trpctool.CallableTool)
	out, err := ct.Call(context.Background(), []byte(`{"file_name":"notes.txt"}`))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(out)
	if !strings.Contains(string(raw), `"deleted":true`) {
		t.Fatalf("%s", raw)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("file still exists")
	}
}

func TestDeleteFileRefusesDirectory(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	tool := NewTool(dir, loggateway.NewNoop())
	ct := tool.(trpctool.CallableTool)
	_, err := ct.Call(context.Background(), []byte(`{"file_name":"sub"}`))
	if err == nil || !strings.Contains(err.Error(), "directory") {
		t.Fatalf("err=%v", err)
	}
}

func TestDeleteFileRefusesOutsideWorkspace(t *testing.T) {
	dir := t.TempDir()
	tool := NewTool(dir, loggateway.NewNoop())
	ct := tool.(trpctool.CallableTool)
	_, err := ct.Call(context.Background(), []byte(`{"file_name":"..\\windows\\temp.txt"}`))
	if err == nil {
		t.Fatal("expected outside-workspace error")
	}
}

func TestDeleteFileRefusesGit(t *testing.T) {
	dir := t.TempDir()
	git := filepath.Join(dir, ".git")
	if err := os.Mkdir(git, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := filepath.Join(git, "config")
	if err := os.WriteFile(cfg, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	tool := NewTool(dir, loggateway.NewNoop())
	ct := tool.(trpctool.CallableTool)
	_, err := ct.Call(context.Background(), []byte(`{"file_name":".git/config"}`))
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "git") {
		t.Fatalf("err=%v", err)
	}
}
