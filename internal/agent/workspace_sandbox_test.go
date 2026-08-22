package agent

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"aranea-agents/internal/biz"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

func sandboxAgent(profile, allowJSON string) biz.Agent {
	return biz.Agent{
		AgentKey: "coder",
		Settings: &biz.AgentRuntimeSettings{
			ToolsEnabled:   true,
			ToolsProfile:   profile,
			ToolsAllowJSON: allowJSON,
		},
	}
}

func TestWorkspaceSandbox_ReadOnlyBlocksWrite(t *testing.T) {
	hook := newWorkspaceSandboxBeforeHook(sandboxAgent("read_only", ""), TRPCBuilderDeps{})
	res, err := hook.HandleBeforeTool(context.Background(), &trpctool.BeforeToolArgs{
		ToolName:  "save_file",
		Arguments: []byte(`{"path":"notes.md","content":"x"}`),
	})
	if err != nil {
		t.Fatalf("hook err: %v", err)
	}
	msg, _ := res.CustomResult.(string)
	if !strings.Contains(msg, "read-only") {
		t.Fatalf("CustomResult = %q, want read-only reason", res.CustomResult)
	}
}

func TestWorkspaceSandbox_ReadOnlyAllowsRead(t *testing.T) {
	base := t.TempDir()
	t.Setenv("ARANEA_WORKSPACE_ROOT", base)
	hook := newWorkspaceSandboxBeforeHook(sandboxAgent("read_only", ""), TRPCBuilderDeps{})
	res, err := hook.HandleBeforeTool(context.Background(), &trpctool.BeforeToolArgs{
		ToolName:  "read_file",
		Arguments: []byte(`{"path":"notes.md"}`),
	})
	if err != nil {
		t.Fatalf("hook err: %v", err)
	}
	if res != nil && res.CustomResult != nil {
		t.Fatalf("read must pass, got CustomResult=%v", res.CustomResult)
	}
}

func TestWorkspaceSandbox_WriteOutsideRootRejected(t *testing.T) {
	base := t.TempDir()
	t.Setenv("ARANEA_WORKSPACE_ROOT", base)
	outside := filepath.Join(t.TempDir(), "escape.txt")
	hook := newWorkspaceSandboxBeforeHook(sandboxAgent("research", `["save_file"]`), TRPCBuilderDeps{})
	args, _ := json.Marshal(map[string]any{"path": outside, "content": "x"})
	res, err := hook.HandleBeforeTool(context.Background(), &trpctool.BeforeToolArgs{
		ToolName:  "save_file",
		Arguments: args,
	})
	if err != nil {
		t.Fatalf("hook err: %v", err)
	}
	msg, _ := res.CustomResult.(string)
	if !strings.Contains(msg, "outside workspace") {
		t.Fatalf("CustomResult = %q, want outside-workspace reason", res.CustomResult)
	}
}

func TestWorkspaceSandbox_WriteInsideRootAllowed(t *testing.T) {
	base := t.TempDir()
	t.Setenv("ARANEA_WORKSPACE_ROOT", base)
	hook := newWorkspaceSandboxBeforeHook(sandboxAgent("research", `["save_file"]`), TRPCBuilderDeps{})
	res, err := hook.HandleBeforeTool(context.Background(), &trpctool.BeforeToolArgs{
		ToolName:  "save_file",
		Arguments: []byte(`{"path":"notes.md","content":"x"}`),
	})
	if err != nil {
		t.Fatalf("hook err: %v", err)
	}
	if res != nil && res.CustomResult != nil {
		t.Fatalf("in-root write must pass, got CustomResult=%v", res.CustomResult)
	}
}

func TestWorkspaceSandbox_ShellBlockedInReadOnly(t *testing.T) {
	hook := newWorkspaceSandboxBeforeHook(sandboxAgent("read_only", ""), TRPCBuilderDeps{})
	res, err := hook.HandleBeforeTool(context.Background(), &trpctool.BeforeToolArgs{
		ToolName:  "shell_exec",
		Arguments: []byte(`{"command":"echo hi"}`),
	})
	if err != nil {
		t.Fatalf("hook err: %v", err)
	}
	msg, _ := res.CustomResult.(string)
	if !strings.Contains(msg, "read-only") {
		t.Fatalf("CustomResult = %q", res.CustomResult)
	}
}

func TestExtractSandboxPath(t *testing.T) {
	t.Parallel()
	if got := extractSandboxPath([]byte(`{"path":" a.md "}`)); got != "a.md" {
		t.Fatalf("path = %q", got)
	}
	if got := extractSandboxPath([]byte(`{"cwd":"C:\\tmp"}`)); !strings.Contains(got, "tmp") {
		t.Fatalf("cwd = %q", got)
	}
	if got := extractSandboxPath([]byte(`{"target":"weixin"}`)); got != "" {
		t.Fatalf("target must not be treated as a filesystem path, got %q", got)
	}
}

func TestWorkspaceSandboxHookPriority(t *testing.T) {
	t.Parallel()
	hook := newWorkspaceSandboxBeforeHook(sandboxAgent("read_only", ""), TRPCBuilderDeps{})
	if hook.Priority() != workspaceSandboxHookPriority {
		t.Fatalf("priority = %d, want %d", hook.Priority(), workspaceSandboxHookPriority)
	}
	if workspaceSandboxHookPriority >= 10 {
		t.Fatal("sandbox must run before tool confirmation (priority 10)")
	}
}
