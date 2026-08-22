package agent

import (
	"context"
	"encoding/json"
	"strings"

	"aranea-agents/internal/agent/callbacks"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// shellOnFailureStateKey marks this invocation so the next shell_exec needs
// a confirmation card (F2). Codex asks again after a failed exec; Aranea
// keeps E1 skip-confirm for the first safe command, then arms this flag.
const shellOnFailureStateKey = "aranea:shell_on_failure"

const shellOnFailureAfterHookPriority = 4

func newShellOnFailureAfterHook() callbacks.Callback {
	return callbacks.NewAfterToolHook(shellOnFailureAfterHookPriority, func(ctx context.Context, args *trpctool.AfterToolArgs) (*trpctool.AfterToolResult, error) {
		recordShellOnFailure(ctx, args)
		return &trpctool.AfterToolResult{}, nil
	})
}

func recordShellOnFailure(ctx context.Context, args *trpctool.AfterToolArgs) {
	if args == nil || !isShellExecRuntimeName(args.ToolName) {
		return
	}
	inv, ok := trpcagent.InvocationFromContext(ctx)
	if !ok || inv == nil {
		return
	}
	if args.Error != nil {
		inv.SetState(shellOnFailureStateKey, true)
		return
	}
	code, ok := extractToolExitCode(args.Result)
	if !ok {
		return
	}
	inv.SetState(shellOnFailureStateKey, code != 0)
}

func shellOnFailureArmed(ctx context.Context) bool {
	inv, ok := trpcagent.InvocationFromContext(ctx)
	if !ok || inv == nil {
		return false
	}
	v, ok := inv.GetState(shellOnFailureStateKey)
	if !ok {
		return false
	}
	armed, _ := v.(bool)
	return armed
}

func extractToolExitCode(result any) (int, bool) {
	m, ok := result.(map[string]any)
	if !ok || m == nil {
		return 0, false
	}
	if code, ok := coerceExitCode(m["exit_code"]); ok {
		return code, true
	}
	for _, key := range []string{"content", "preview_head"} {
		s, isStr := m[key].(string)
		if !isStr || strings.TrimSpace(s) == "" {
			continue
		}
		var inner map[string]any
		if json.Unmarshal([]byte(s), &inner) != nil {
			continue
		}
		if code, ok := coerceExitCode(inner["exit_code"]); ok {
			return code, true
		}
	}
	return 0, false
}

func coerceExitCode(v any) (int, bool) {
	if v == nil {
		return 0, false
	}
	switch n := v.(type) {
	case int:
		return n, true
	case int32:
		return int(n), true
	case int64:
		return int(n), true
	case float64:
		return int(n), true
	case json.Number:
		i, err := n.Int64()
		if err != nil {
			return 0, false
		}
		return int(i), true
	default:
		return 0, false
	}
}
