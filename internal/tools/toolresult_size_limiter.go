package tools

import (
	"context"
	"fmt"
	"unicode/utf8"

	"aranea-agents/pkg/loggateway"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

const (
	// defaultTruncationMarker is appended when a tool result is truncated.
	defaultTruncationMarker = "\n\n[output truncated: exceeded %d character limit, original size: %d characters]"
)

// NewOutputSizeLimiterHook creates an AfterToolCallbackStructured that
// truncates tool result strings exceeding maxChars (measured in UTF-8 runes).
// When the result is a string that exceeds the limit, it is truncated and a
// marker is appended indicating the limit and original byte size.
//
// Non-string results and results within the limit are passed through unchanged.
// Tool execution errors are also passed through unchanged.
//
// This is a simpler alternative to ToolResultGate — no blob persistence, just
// in-place truncation. Suitable for framework contribution where the full
// result is not needed after the LLM processes it.
func NewOutputSizeLimiterHook(maxChars int, lg loggateway.Logger) trpctool.AfterToolCallbackStructured {
	if maxChars <= 0 {
		maxChars = 50000
	}
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return func(ctx context.Context, args *trpctool.AfterToolArgs) (*trpctool.AfterToolResult, error) {
		if args == nil || args.Error != nil {
			// Pass through errors unchanged
			return &trpctool.AfterToolResult{}, nil
		}

		resultStr, ok := args.Result.(string)
		if !ok {
			// Non-string result — pass through unchanged
			return &trpctool.AfterToolResult{}, nil
		}

		if utf8.RuneCountInString(resultStr) <= maxChars {
			// Within limit — pass through unchanged
			return &trpctool.AfterToolResult{}, nil
		}

		originalSize := utf8.RuneCountInString(resultStr)
		truncated := truncateRunes(resultStr, maxChars)
		marker := fmt.Sprintf(defaultTruncationMarker, maxChars, originalSize)
		newResult := truncated + marker

		lg.Info("tool output truncated",
			loggateway.StepID("tool.output_limiter"),
			loggateway.Str("tool", args.ToolName),
			loggateway.Str("tool_call_id", args.ToolCallID),
			loggateway.Int("original_size", originalSize),
			loggateway.Int("max_chars", maxChars),
		)

		return &trpctool.AfterToolResult{
			CustomResult: newResult,
		}, nil
	}
}

// truncateRunes truncates s to at most maxChars runes, preserving valid UTF-8.
// Unlike the package-level truncateString (which appends a gate-specific
// marker), this function returns the raw truncated prefix without any suffix.
func truncateRunes(s string, maxChars int) string {
	runes := []rune(s)
	if len(runes) <= maxChars {
		return s
	}
	return string(runes[:maxChars])
}
