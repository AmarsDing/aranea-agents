package tools

import (
	"context"
	"fmt"
	"unicode/utf8"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// NewToolResultGateAfterHook creates an AfterToolCallbackStructured that
// applies ToolResultGate logic after tool execution. If the result is a
// string exceeding the gate's threshold, the gate's Check method is invoked
// to persist the full content and replace it with a preview.
//
// This adapter bridges the project's biz.ToolResultGate with the framework's
// tool.AfterToolCallbackStructured callback interface.
func NewToolResultGateAfterHook(gate *biz.ToolResultGate, ag biz.Agent, lg loggateway.Logger) trpctool.AfterToolCallbackStructured {
	return func(ctx context.Context, args *trpctool.AfterToolArgs) (*trpctool.AfterToolResult, error) {
		if args == nil || gate == nil {
			return &trpctool.AfterToolResult{Context: ctx}, nil
		}
		// Only process successful string results.
		if args.Error != nil || args.Result == nil {
			return &trpctool.AfterToolResult{Context: ctx}, nil
		}

		content, ok := args.Result.(string)
		if !ok {
			return &trpctool.AfterToolResult{Context: ctx}, nil
		}
		if utf8.RuneCountInString(content) <= biz.ToolResultSizeThreshold {
			return &trpctool.AfterToolResult{Context: ctx}, nil
		}

		sessionID := sessionIDFromContext(ctx)
		if sessionID == "" {
			// No session context: truncate as fallback.
			lg.Warn("ToolResultGate after-hook: no session ID in context, truncating as fallback",
				loggateway.StepID("tool.result_gate.no_session"),
				loggateway.Str("tool", args.ToolName),
			)
			preview := truncateString(content, biz.ToolResultPreviewSize)
			return &trpctool.AfterToolResult{
				Context:      ctx,
				CustomResult: preview,
			}, nil
		}

		toolCallID := args.ToolCallID
		if toolCallID == "" {
			toolCallID = "auto"
		}

		result, err := gate.Check(ctx, sessionID, toolCallID, args.ToolName, "", content, 0)
		if err != nil {
			lg.Error("ToolResultGate after-hook: gate.Check failed, truncating as fallback",
				loggateway.StepID("tool.result_gate.check_fail"),
				loggateway.Str("tool", args.ToolName),
				loggateway.Str("session_id", sessionID),
				loggateway.Err(err),
			)
			preview := truncateString(content, biz.ToolResultPreviewSize)
			return &trpctool.AfterToolResult{
				Context:      ctx,
				CustomResult: preview,
			}, nil
		}

		if result.DidPersist {
			return &trpctool.AfterToolResult{
				Context:      ctx,
				CustomResult: result.PreviewText,
			}, nil
		}

		return &trpctool.AfterToolResult{Context: ctx}, nil
	}
}

// sessionIDFromContext extracts a session ID from context.
// This is a minimal implementation for the adapter layer; the production
// code in internal/agent uses trpcagent.InvocationFromContext for richer
// session extraction.
func sessionIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(sessionIDCtxKey{}).(string); ok {
		return v
	}
	return ""
}

type sessionIDCtxKey struct{}

// WithSessionID returns a context with the given session ID for use with
// the ToolResultGate after-hook.
func WithSessionID(ctx context.Context, sessionID string) context.Context {
	return context.WithValue(ctx, sessionIDCtxKey{}, sessionID)
}

// truncateString truncates a string to at most maxRunes runes and appends
// a truncation notice.
func truncateString(s string, maxRunes int) string {
	runeCount := utf8.RuneCountInString(s)
	if runeCount <= maxRunes {
		return s
	}
	runes := []rune(s)
	head := string(runes[:maxRunes])
	return head + fmt.Sprintf("\n\n... [truncated %d → %d chars, persist failed] ...", runeCount, maxRunes)
}
