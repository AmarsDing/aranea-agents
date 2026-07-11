package agent

import (
	"context"
	"strings"

	"aranea-agents/internal/agent/callbacks"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/tools/memory"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// compactToolName is the name of the agent-invocable compact tool.
const compactToolName = "compact"

// newCompactContextBeforeHook returns a BeforeToolHook that injects
// ManualCompressor and sessionID into the tool execution context for
// the compact tool.
//
// Returns nil when ManualCompressor is not wired (the tool is registered
// but will fail with "ManualCompressor not available" if invoked).
func newCompactContextBeforeHook(_ biz.Agent, deps TRPCBuilderDeps) callbacks.Callback {
	if deps.ManualCompressor == nil {
		return nil
	}
	return callbacks.NewBeforeToolHook(-10, func(ctx context.Context, args *trpctool.BeforeToolArgs) (*trpctool.BeforeToolResult, error) {
		if args == nil || args.Declaration == nil {
			return &trpctool.BeforeToolResult{Context: ctx}, nil
		}
		if args.Declaration.Name != compactToolName {
			return &trpctool.BeforeToolResult{Context: ctx}, nil
		}
		inv, ok := trpcagent.InvocationFromContext(ctx)
		if !ok || inv == nil || inv.Session == nil {
			return &trpctool.BeforeToolResult{Context: ctx}, nil
		}
		sessID := strings.TrimSpace(inv.Session.ID)
		if sessID == "" {
			return &trpctool.BeforeToolResult{Context: ctx}, nil
		}
		ctx = memory.WithManualCompressor(ctx, deps.ManualCompressor)
		ctx = memory.WithCompactSessionID(ctx, sessID)
		return &trpctool.BeforeToolResult{Context: ctx}, nil
	})
}
