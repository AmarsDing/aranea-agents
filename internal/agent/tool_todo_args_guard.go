package agent

import (
	"context"
	"encoding/json"

	"aranea-agents/internal/agent/callbacks"
	"aranea-agents/pkg/loggateway"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// newTodoArgsGuardBeforeHook strips the "todos" parameter from non-todo_write
// tool calls. The LLM occasionally injects a "todos" field into unrelated tool
// calls (e.g. read_file, exec_command), which causes those tools to fail or
// timeout — the root cause of "stuck tool" incidents.
func newTodoArgsGuardBeforeHook(lg loggateway.Logger) callbacks.BeforeToolHook {
	return callbacks.NewBeforeToolHook(2, func(ctx context.Context, args *trpctool.BeforeToolArgs) (*trpctool.BeforeToolResult, error) {
		if args == nil || len(args.Arguments) == 0 {
			return &trpctool.BeforeToolResult{Context: ctx}, nil
		}
		// Only guard non-todo_write tools
		if args.ToolName == "todo_write" {
			return &trpctool.BeforeToolResult{Context: ctx}, nil
		}
		var payload map[string]json.RawMessage
		if json.Unmarshal(args.Arguments, &payload) != nil {
			return &trpctool.BeforeToolResult{Context: ctx}, nil
		}
		if _, hasTodos := payload["todos"]; !hasTodos {
			return &trpctool.BeforeToolResult{Context: ctx}, nil
		}
		delete(payload, "todos")
		b, err := json.Marshal(payload)
		if err != nil {
			return &trpctool.BeforeToolResult{Context: ctx}, nil
		}
		if lg != nil {
			lg.Warn("stripped todos arg from non-todo_write tool",
				loggateway.Str("tool", args.ToolName),
			)
		}
		// NOTE: This hook mutates args.Arguments in-place.
		// It must execute before other hooks that read Arguments (e.g. system-args guard at priority 3).
		args.Arguments = b
		return &trpctool.BeforeToolResult{Context: ctx}, nil
	})
}
