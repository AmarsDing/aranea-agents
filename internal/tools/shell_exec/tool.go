// Package shell_exec exposes a constrained local shell tool for ADK / diagnostics (e.g. ipconfig).
package shell_exec

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"aranea-agents/internal/tools/argmap"
	"aranea-agents/internal/tools/workspace"

	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
)

const maxOutputRunes = 256 * 1024
const runTimeout = 60 * time.Second

type shellArgs struct {
	Command    string `json:"command"`
	WorkingDir string `json:"working_dir,omitempty"`
}

func clip(s string) string {
	s = strings.TrimSpace(s)
	r := []rune(s)
	if len(r) > maxOutputRunes {
		return string(r[:maxOutputRunes]) + "\n... (truncated)"
	}
	return s
}

func validateCommand(cmd string) error {
	lower := strings.ToLower(cmd)
	patterns := []string{
		"rm -rf", "del /s", "del /q", "format ",
		"mkfs", ":(){",
		"powershell -enc", "powershell -e ",
		" | sh", "& del ", "chmod 777",
		"drop table", "truncate table",
	}
	for _, p := range patterns {
		if strings.Contains(lower, p) {
			return fmt.Errorf("command blocked by safety policy (pattern %q)", p)
		}
	}
	return nil
}

// Run executes one shell command with cwd constrained to the workspace sandbox when working_dir is set;
// if working_dir is empty, cwd defaults to the workspace root.
func Run(ctx context.Context, argsMap map[string]any) (map[string]any, error) {
	cmd := strings.TrimSpace(argmap.String(argsMap, "command"))
	if cmd == "" {
		return nil, fmt.Errorf("command is required")
	}
	if err := validateCommand(cmd); err != nil {
		return nil, err
	}

	wdRaw := strings.TrimSpace(argmap.String(argsMap, "working_dir"))
	var dir string
	if wdRaw != "" {
		abs, _, err := workspace.ResolvePath(wdRaw)
		if err != nil {
			return nil, err
		}
		dir = abs
	} else {
		root, err := workspace.Root()
		if err != nil {
			return nil, err
		}
		dir = root
	}

	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, runTimeout)
	defer cancel()

	var c *exec.Cmd
	if runtime.GOOS == "windows" {
		// Prefer UTF-8 code page for tools that honor it; localized stderr still decoded via GBK fallback.
		wrapped := "chcp 65001>nul && " + cmd
		c = exec.CommandContext(ctx, "cmd.exe", "/C", wrapped)
	} else {
		c = exec.CommandContext(ctx, "sh", "-c", cmd)
	}
	c.Dir = dir

	var stdout, stderr bytes.Buffer
	c.Stdout = &stdout
	c.Stderr = &stderr

	err := c.Run()
	exitCode := 0
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			exitCode = ee.ExitCode()
		} else {
			exitCode = -1
		}
	}

	outStr := clip(bytesToShellText(stdout.Bytes()))
	errStr := clip(bytesToShellText(stderr.Bytes()))
	return map[string]any{
		"exit_code": exitCode,
		"stdout":    outStr,
		"stderr":    errStr,
		"cwd":       dir,
	}, nil
}

const desc = `Run one non-interactive shell command on the server host (Windows: cmd.exe; Unix: sh -c). working_dir is optional and must stay inside the workspace sandbox (relative to ARANEA_WORKSPACE_ROOT / WORKSPACE_ROOT); default cwd is the workspace root. You cannot create folders on the OS user Desktop or other paths outside that sandbox—use a relative path like "mkdir mydir" in the workspace. On Windows the command is run after "chcp 65001" so UTF-8-friendly tools work; localized cmd errors are normalized to UTF-8. Dangerous patterns are rejected.`

// New builds an ADK function tool named shell_exec (matches platform catalog tool_key).
func New() (tool.Tool, error) {
	return NewWithName("shell_exec")
}

// NewWithName builds the same implementation under an alternate function name (e.g. "shell" for model habits).
func NewWithName(name string) (tool.Tool, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "shell_exec"
	}
	return functiontool.New(functiontool.Config{
		Name:        name,
		Description: desc,
	}, func(tc tool.Context, in shellArgs) (map[string]any, error) {
		return Run(tc, map[string]any{
			"command":     in.Command,
			"working_dir": in.WorkingDir,
		})
	})
}
