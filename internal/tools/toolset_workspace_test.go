package tools

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

func TestAssemble_hostexecUsesShellExecDir(t *testing.T) {
	root := t.TempDir()
	ts, err := Assemble(context.Background(), AssemblyConfig{
		EnabledTools: []string{"hostexec"},
		ShellExec:    ShellExecConfig{Dir: root},
	})
	if err != nil {
		t.Fatal(err)
	}
	callable, err := findExecCommand(ts)
	if err != nil {
		t.Fatal(err)
	}
	out, err := callable.Call(context.Background(), []byte(`{"command":"cd"}`))
	if err != nil {
		t.Fatal(err)
	}
	m, ok := out.(map[string]any)
	if !ok {
		t.Fatalf("output type %T", out)
	}
	output, _ := m["output"].(string)
	absRoot, _ := filepath.Abs(root)
	if output != absRoot {
		t.Fatalf("cwd output=%q want %q", output, absRoot)
	}
}

func findExecCommand(ts *AssembledToolsets) (trpctool.CallableTool, error) {
	if ts == nil {
		return nil, os.ErrInvalid
	}
	for _, set := range ts.ToolSets {
		if set == nil {
			continue
		}
		for _, tool := range set.Tools(context.Background()) {
			if tool == nil || tool.Declaration() == nil {
				continue
			}
			if tool.Declaration().Name != "exec_command" {
				continue
			}
			if c, ok := tool.(trpctool.CallableTool); ok {
				return c, nil
			}
		}
	}
	return nil, os.ErrNotExist
}
