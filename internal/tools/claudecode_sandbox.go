package tools

import (
	"context"
	"encoding/json"
	"strings"

	kerrors "github.com/go-kratos/kratos/v2/errors"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
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
	var input struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return nil, kerrors.BadRequest("BASH", "failed to parse command arguments: "+err.Error())
	}
	cmd := strings.TrimSpace(input.Command)
	if cmd == "" {
		return w.inner.Call(ctx, args)
	}
	// Extract the first token (command name) for allowlist matching.
	// This prevents prefix-based bypasses like "gitrm" matching "git".
	cmdName, safe := firstCommandToken(cmd)
	if !safe {
		return nil, kerrors.Forbidden("BASH", "command contains shell metacharacters, chaining is not allowed: "+truncate(cmd, 64))
	}
	allowed := false
	for _, entry := range w.allowList {
		if strings.TrimSpace(entry) == cmdName {
			allowed = true
			break
		}
	}
	if !allowed {
		return nil, kerrors.Forbidden("BASH", "command not in allowlist: "+truncate(cmd, 64))
	}
	return w.inner.Call(ctx, args)
}

// shellMetacharacters lists characters that enable command chaining,
// substitution, or redirection. Any command containing these is rejected
// because the allowlist only validates the first token — a chained command
// like "git status && rm -rf /" would pass the first-token check but execute
// the second command unrestricted.
var shellMetacharacters = []string{
	";", "&&", "||", "|", "&",
	"$(", "`", "${", ">", "<",
	"\n", "\r", "#",
}

// firstCommandToken extracts the first token of a shell command,
// handling common shell operators and pipes.
// Returns the command name and true if the command is safe (no shell
// metacharacters), or empty string and false if injection is detected.
func firstCommandToken(cmd string) (string, bool) {
	// Trim whitespace first; an empty/whitespace-only command is safe.
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return "", true
	}
	// Reject commands containing shell metacharacters that enable chaining
	// or substitution. This prevents bypass via "git status && rm -rf /"
	// or similar injection patterns.
	for _, meta := range shellMetacharacters {
		if strings.Contains(cmd, meta) {
			return "", false
		}
	}
	// Strip leading shell operators by repeatedly trimming known
	// multi-character and single-character prefixes. Unlike TrimLeft,
	// this does not strip arbitrary characters from the cutset — only
	// exact prefix matches are removed, so "||git" becomes "git" not "it".
	for {
		trimmed := false
		for _, prefix := range []string{"||", "&&", "|", "&", ";", "(", ")", "<", ">"} {
			if strings.HasPrefix(cmd, prefix) {
				cmd = strings.TrimPrefix(cmd, prefix)
				trimmed = true
				break
			}
		}
		// Also skip leading whitespace between operators and the command.
		if after := strings.TrimLeft(cmd, " \t"); after != cmd {
			cmd = after
			continue
		}
		if !trimmed {
			break
		}
	}
	// Split by whitespace to get the command name
	fields := strings.Fields(cmd)
	if len(fields) == 0 {
		return "", true
	}
	return fields[0], true
}

func truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n]) + "..."
}
