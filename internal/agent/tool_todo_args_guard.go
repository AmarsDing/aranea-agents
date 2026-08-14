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
		// P1-3: 剥离结果经 ModifiedArguments 返回——框架收到后会更新
		// args.Arguments 再传给链上的下一个 hook（tool/callbacks.go
		// processBeforeToolResult:358），优先级 2→3 的顺序契约不变；
		// 同时这也是写回 toolCall.Function.Arguments 的唯一通道，
		// 原地改 args.Arguments 不会到达工具执行。
		return &trpctool.BeforeToolResult{Context: ctx, ModifiedArguments: b}, nil
	})
}
