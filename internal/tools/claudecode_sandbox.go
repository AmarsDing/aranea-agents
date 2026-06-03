package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
	trpcclaudecode "trpc.group/trpc-go/trpc-agent-go/tool/claudecode"
)

// ClaudeCodeSandboxConfig defines security constraints for ClaudeCode tools.
type ClaudeCodeSandboxConfig struct {
	// BaseDir restricts file operations to this directory.
	// When set, path traversal outside BaseDir is blocked.
	BaseDir string
	// ReadOnly disables Write/Edit/NotebookEdit tools.
	ReadOnly bool
	// CommandAllowList restricts shell commands to this list.
	// When empty, no command restriction is applied.
	CommandAllowList []string
}

// ApplySandboxOptions converts sandbox config to claudecode ToolSet options.
// Returns the options slice with sandbox-relevant options appended.
func ApplySandboxOptions(cfg ClaudeCodeSandboxConfig, opts []trpcclaudecode.Option) []trpcclaudecode.Option {
	if cfg.BaseDir != "" {
		opts = append(opts, trpcclaudecode.WithBaseDir(cfg.BaseDir))
	}
	if cfg.ReadOnly {
		opts = append(opts, trpcclaudecode.WithReadOnly(true))
	}
	return opts
}

// SandboxedToolSet wraps a ToolSet to enforce command allowlist on the bash tool.
// If CommandAllowList is empty, the original ToolSet is returned unchanged.
func SandboxedToolSet(ts trpctool.ToolSet, cfg ClaudeCodeSandboxConfig) trpctool.ToolSet {
	if len(cfg.CommandAllowList) == 0 {
		return ts
	}
	tools := ts.Tools(context.Background())
	var replaced bool
	var out []trpctool.Tool
	for _, t := range tools {
		if t.Declaration() != nil && t.Declaration().Name == "bash" {
			callable, ok := t.(trpctool.CallableTool)
			if !ok {
				out = append(out, t)
				continue
			}
			out = append(out, &whitelistedBashTool{inner: callable, allowList: cfg.CommandAllowList})
			replaced = true
		} else {
			out = append(out, t)
		}
	}
	if !replaced {
		return ts
	}
	return &sandboxedToolSet{inner: ts, tools: out}
}

type sandboxedToolSet struct {
	inner trpctool.ToolSet
	tools []trpctool.Tool
}

func (s *sandboxedToolSet) Tools(_ context.Context) []trpctool.Tool { return s.tools }
func (s *sandboxedToolSet) Name() string                            { return s.inner.Name() }
func (s *sandboxedToolSet) Close() error                            { return s.inner.Close() }

type whitelistedBashTool struct {
	inner     trpctool.CallableTool
	allowList []string
}

func (w *whitelistedBashTool) Declaration() *trpctool.Declaration {
	return w.inner.Declaration()
}

func (w *whitelistedBashTool) Call(ctx context.Context, args []byte) (any, error) {
	// Parse the command from args to check against allowlist.
	// The args are JSON with a "command" field.
	var input struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return w.inner.Call(ctx, args)
	}
	cmd := strings.TrimSpace(input.Command)
	if cmd == "" {
		return w.inner.Call(ctx, args)
	}
	// Check if the command starts with any allowed prefix.
	allowed := false
	for _, prefix := range w.allowList {
		if strings.HasPrefix(cmd, strings.TrimSpace(prefix)) {
			allowed = true
			break
		}
	}
	if !allowed {
		return nil, fmt.Errorf("bash: command not in allowlist: %s", truncate(cmd, 64))
	}
	return w.inner.Call(ctx, args)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
