package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"aranea-agents/internal/agent/callbacks"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// fileMutatingToolNames are tools whose execution modifies workspace files.
// Reminders fire when such edits are not followed by a test/command run
// containing "test" (the tool side-effect feedback loop from Grok Build).
var fileMutatingToolNames = []string{"write", "edit", "patch", "delete", "create", "rename", "move"}

// commandToolNames are tools that execute shell commands; a command
// containing "test" counts as a verification run that clears reminders.
var commandToolNames = []string{"exec_command", "shell_exec", "bash", "run_command", "run_test"}

func isFileMutatingTool(name string) bool {
	l := strings.ToLower(name)
	for _, p := range fileMutatingToolNames {
		if strings.Contains(l, p) {
			return true
		}
	}
	return false
}

func isCommandTool(name string) bool {
	l := strings.ToLower(name)
	for _, p := range commandToolNames {
		if l == p {
			return true
		}
	}
	return false
}

func commandContainsTest(params map[string]string) bool {
	for _, key := range []string{"command", "cmd", "script", "args"} {
		if v, ok := params[key]; ok && strings.Contains(strings.ToLower(v), "test") {
			return true
		}
	}
	return false
}

// ToolReminder tracks file mutations and emits reminders when edits are not
// followed by a verification (test) run. Pure in-memory, goroutine-safe.
type ToolReminder struct {
	mu          sync.Mutex
	unverified  []string // paths edited since last test run
	maxReminder int      // cap on tracked paths to bound memory
}

func NewToolReminder() *ToolReminder {
	return &ToolReminder{maxReminder: 20}
}

// OnToolExecuted records one tool execution. File-mutating tools arm the
// reminder; command tools containing "test" clear it.
func (r *ToolReminder) OnToolExecuted(name string, params map[string]string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	switch {
	case isFileMutatingTool(name):
		path := params["path"]
		if path == "" {
			path = params["file"]
		}
		if path == "" {
			path = name // fall back to tool name when no path param exists
		}
		if len(r.unverified) < r.maxReminder {
			r.unverified = append(r.unverified, path)
		}
	case isCommandTool(name) && commandContainsTest(params):
		r.unverified = nil
	}
}

// Collect returns pending reminders (empty when all edits are verified).
func (r *ToolReminder) Collect() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.unverified) == 0 {
		return nil
	}
	return []string{fmt.Sprintf(
		"Files modified without running tests: %s. Consider running tests to verify the changes.",
		strings.Join(r.unverified, ", "),
	)}
}

// toolReminderStateKey is the invocation state key holding the per-run
// ToolReminder instance. The instance is created per invocation (see
// newToolReminderBeforeAgentHook) so concurrent sessions sharing a cached
// Agent never share reminder state.
const toolReminderStateKey = "aranea.tool_reminder"

// newToolReminderBeforeAgentHook creates a fresh ToolReminder for every
// invocation. The companion AfterTool hook (newToolReminderAfterHook)
// consumes it from invocation state, scoping reminders per run instead of
// per cached Agent.
func newToolReminderBeforeAgentHook() callbacks.Callback {
	return callbacks.NewBeforeAgentHook(0, func(ctx context.Context, args *trpcagent.BeforeAgentArgs) (*trpcagent.BeforeAgentResult, error) {
		if args != nil && args.Invocation != nil {
			args.Invocation.SetState(toolReminderStateKey, NewToolReminder())
		}
		return &trpcagent.BeforeAgentResult{Context: ctx}, nil
	})
}

// toolReminderFromInvocation resolves the per-invocation ToolReminder,
// lazily creating one when the BeforeAgent hook did not run (defensive).
func toolReminderFromInvocation(inv *trpcagent.Invocation) *ToolReminder {
	if v, found := inv.GetState(toolReminderStateKey); found {
		if r, ok := v.(*ToolReminder); ok && r != nil {
			return r
		}
	}
	r := NewToolReminder()
	inv.SetState(toolReminderStateKey, r)
	return r
}

// newToolReminderAfterHook appends pending reminders to tool results so the
// LLM sees the side-effect feedback in the tool response itself.
// Non-command results carry the reminder; test runs clear it.
func newToolReminderAfterHook() callbacks.Callback {
	return callbacks.NewAfterToolHook(60, func(ctx context.Context, args *trpctool.AfterToolArgs) (*trpctool.AfterToolResult, error) {
		if args == nil {
			return &trpctool.AfterToolResult{}, nil
		}
		inv, ok := trpcagent.InvocationFromContext(ctx)
		if !ok || inv == nil {
			return &trpctool.AfterToolResult{}, nil
		}
		reminder := toolReminderFromInvocation(inv)
		params := parseToolArgsParams(args.Arguments)
		reminder.OnToolExecuted(args.ToolName, params)
		reminders := reminder.Collect()
		if len(reminders) == 0 {
			return &trpctool.AfterToolResult{}, nil
		}
		// Do not nag on the verification run itself or on read-only tools when
		// the reminder would drown a small result; append to substantial results.
		if isCommandTool(args.ToolName) {
			return &trpctool.AfterToolResult{}, nil
		}
		note := "\n\n[reminder] " + strings.Join(reminders, " ")
		switch v := args.Result.(type) {
		case string:
			return &trpctool.AfterToolResult{CustomResult: v + note}, nil
		case []byte:
			return &trpctool.AfterToolResult{CustomResult: append(v, []byte(note)...)}, nil
		default:
			// Non-text results: leave untouched; the reminder stays armed for
			// the next text result.
			return &trpctool.AfterToolResult{}, nil
		}
	})
}

// parseToolArgsParams flattens a JSON argument object into a string map
// (best-effort; non-string values are formatted).
func parseToolArgsParams(raw []byte) map[string]string {
	out := make(map[string]string)
	if len(raw) == 0 {
		return out
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return out
	}
	for k, v := range m {
		switch tv := v.(type) {
		case string:
			out[k] = tv
		default:
			out[k] = fmt.Sprintf("%v", tv)
		}
	}
	return out
}
