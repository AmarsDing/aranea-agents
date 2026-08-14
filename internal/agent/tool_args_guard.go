package agent

import (
	"context"
	"encoding/json"
	"strings"

	"aranea-agents/internal/agent/callbacks"
	"aranea-agents/pkg/loggateway"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// systemOnlyToolArgKeys must not be supplied by the model or frontend (23 tools.md §0.3).
var systemOnlyToolArgKeys = map[string]struct{}{
	"tenant_id": {}, "agent_id": {}, "workspace_id": {},
	"session_id": {}, "request_id": {}, "user_id": {},
}

func newToolArgsGuardBeforeHook(lg loggateway.Logger) callbacks.BeforeToolHook {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	logger := lg
	return callbacks.NewBeforeToolHook(3, func(ctx context.Context, args *trpctool.BeforeToolArgs) (*trpctool.BeforeToolResult, error) {
		if args == nil || len(args.Arguments) == 0 {
			return &trpctool.BeforeToolResult{Context: ctx}, nil
		}
		var payload map[string]any
		if err := json.Unmarshal(args.Arguments, &payload); err != nil {
			// Non-JSON arguments: log a warning rather than silently
			// passing them through, since system-only keys could be present
			// in non-standard formats that bypass the guard.
			toolName := ""
			if args != nil {
				toolName = args.ToolName
			}
			logger.Warn("tool args guard: non-JSON arguments, cannot strip system keys",
				loggateway.StepID("tool.args_guard"),
				loggateway.Str("tool", toolName))
			return &trpctool.BeforeToolResult{Context: ctx}, nil
		}
		changed := false
		for k := range payload {
			if _, blocked := systemOnlyToolArgKeys[strings.ToLower(strings.TrimSpace(k))]; blocked {
				delete(payload, k)
				changed = true
			}
		}
		if !changed {
			return &trpctool.BeforeToolResult{Context: ctx}, nil
		}
		b, err := json.Marshal(payload)
		if err != nil {
			logger.Warn("tool args guard: failed to re-marshal cleaned args",
				loggateway.StepID("tool.args_guard"),
				loggateway.Err(err))
			return &trpctool.BeforeToolResult{Context: ctx}, nil
		}
		// P1-3: 清洗结果必须经 ModifiedArguments 返回，框架才会写回
		// 实际执行的 toolCall.Function.Arguments（原地改 args.Arguments 无效）。
		return &trpctool.BeforeToolResult{Context: ctx, ModifiedArguments: b}, nil
	})
}
