package agent

import (
	"context"
	"encoding/json"
	"strings"

	"aranea-agents/internal/agent/callbacks"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// systemOnlyToolArgKeys must not be supplied by the model or frontend (23 tools.md §0.3).
var systemOnlyToolArgKeys = map[string]struct{}{
	"tenant_id": {}, "agent_id": {}, "workspace_id": {},
	"session_id": {}, "request_id": {}, "user_id": {},
}

func newToolArgsGuardBeforeHook() callbacks.BeforeToolHook {
	return callbacks.NewBeforeToolHook(3, func(ctx context.Context, args *trpctool.BeforeToolArgs) (*trpctool.BeforeToolResult, error) {
		if args == nil || len(args.Arguments) == 0 {
			return &trpctool.BeforeToolResult{Context: ctx}, nil
		}
		var payload map[string]any
		if json.Unmarshal(args.Arguments, &payload) != nil {
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
			return &trpctool.BeforeToolResult{Context: ctx}, nil
		}
		args.Arguments = b
		return &trpctool.BeforeToolResult{Context: ctx}, nil
	})
}
