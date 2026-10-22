package tools

import (
	"context"
	"strings"
	"unicode/utf8"

	"aranea-agents/internal/toolspill"
	"aranea-agents/pkg/loggateway"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

const (
	// defaultTruncationMarker is appended when a tool result is truncated.
	defaultTruncationMarker = "\n\n[output truncated: exceeded %d character limit, original size: %d characters]"
)

// NewOutputSizeLimiterHook creates an AfterToolCallbackStructured that
// truncates tool result strings exceeding maxChars (measured in UTF-8 runes).
// Oversized strings become the same truncation envelope as ToolDecorator
// (truncated / original_size / total_lines / mode / content) so MCP and
// deferred tools surface a line count to the model.
//
// This is a fallback for tools that skip ToolDecorator (deferred / MCP).
// Decorator-governed results are left alone: truncation/offload envelopes,
// tools with a builtin ResultBudget override, and strings already carrying
// the limiter marker. Non-string results, in-budget strings, and execution
// errors also pass through unchanged.
//
// P0-2 spill gate (batch-A gray switch, design §3.4): when a toolspill.Deps
// with a non-nil Store is supplied via spillOpt, an oversized string is first
// preserved whole through the workspace spill store and replaced by a locator
// envelope; if the spill write itself fails (after one retry) the content is
// preserved with an explicit enforcement=partial note rather than being
// silently truncated. With no deps the legacy hard-truncation path runs
// byte-identically.
func NewOutputSizeLimiterHook(maxChars int, lg loggateway.Logger, spillOpt ...*toolspill.Deps) trpctool.AfterToolCallbackStructured {
	if maxChars <= 0 {
		maxChars = 50000
	}
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	var spill *toolspill.Deps
	if len(spillOpt) > 0 {
		spill = spillOpt[0]
	}
	return func(ctx context.Context, args *trpctool.AfterToolArgs) (*trpctool.AfterToolResult, error) {
		if args == nil || args.Error != nil {
			// Pass through errors unchanged
			return &trpctool.AfterToolResult{}, nil
		}
		if outputLimiterShouldSkip(args.ToolName, args.Result) {
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

		if envelope, ok := spillOversizedString(ctx, spill, lg, args.ToolName, resultStr, originalSize); ok {
			return &trpctool.AfterToolResult{CustomResult: envelope}, nil
		}

		truncated := truncateRunes(resultStr, maxChars)

		lg.Info("tool output truncated",
			loggateway.StepID("tool.output_limiter"),
			loggateway.Str("tool", args.ToolName),
			loggateway.Str("tool_call_id", args.ToolCallID),
			loggateway.Int("original_size", originalSize),
			loggateway.Int("total_lines", countTotalLines([]byte(resultStr))),
			loggateway.Int("max_chars", maxChars),
		)

		return &trpctool.AfterToolResult{
			CustomResult: map[string]any{
				"truncated":     true,
				"original_size": originalSize,
				"total_lines":   countTotalLines([]byte(resultStr)),
				"mode":          "head",
				"content":       truncated,
			},
		}, nil
	}
}

// spillOversizedString is the P0-2 spill branch of the AfterTool size
// limiter. It mirrors ToolDecorator.spillOversizedResult for the undecorated
// string path (deferred / MCP tools): spill the full payload and hand the
// model a locator pointer; fail-soft preserves the content with an explicit
// enforcement=partial note instead of truncating. Returns (nil, false) when
// the gate is off or no session identity is available, leaving the legacy
// truncation path untouched.
func spillOversizedString(ctx context.Context, deps *toolspill.Deps, lg loggateway.Logger, toolName, resultStr string, originalSize int) (map[string]any, bool) {
	if !deps.Enabled() {
		return nil, false
	}
	sessionID := ""
	if inv, ok := trpcagent.InvocationFromContext(ctx); ok && inv != nil && inv.Session != nil {
		sessionID = inv.Session.ID
	}
	if sessionID == "" {
		// Spill records are session-scoped; without a session there is no
		// safe ownership key — keep the legacy truncation envelope.
		return nil, false
	}
	loc, err := toolspill.PutWithRetry(ctx, deps.Store, toolspill.Record{
		SessionID: sessionID,
		Tool:      toolName,
		Content:   []byte(resultStr),
	})
	if err != nil {
		lg.Warn("tool output spill failed, preserving full output (enforcement=partial)",
			loggateway.StepID("tool.output_limiter.spill"),
			loggateway.Str("tool", toolName),
			loggateway.Int("original_size", originalSize),
			loggateway.Err(err),
		)
		deps.ReportSpillFailed(ctx, lg, "tool.output_limiter.spill", sessionID, err)
		return map[string]any{
			"truncated":     false,
			"spill_failed":  true,
			"original_size": originalSize,
			"total_lines":   countTotalLines([]byte(resultStr)),
			"mode":          "preserved",
			"content":       resultStr + toolspill.PartialPreservedNote,
		}, true
	}
	lg.Info("tool output spilled",
		loggateway.StepID("tool.output_limiter.spill"),
		loggateway.Str("tool", toolName),
		loggateway.Int("original_size", originalSize),
		loggateway.Str("locator", loc.String()),
	)
	return map[string]any{
		"truncated":     true,
		"spilled":       true,
		"original_size": originalSize,
		"total_lines":   countTotalLines([]byte(resultStr)),
		"mode":          "spill",
		"content":       toolspill.PointerText(toolName, len(resultStr), loc),
		"locator":       loc.String(),
		"read_hint":     loc.RetrievalHint(),
	}, true
}

// isSizeGovernedResult reports whether result is a decorator truncation or
// offload envelope. Those maps already encode the size contract for the LLM.
// Spill envelopes (P0-2) are likewise already size-governed: spilled=true
// carries the locator pointer, spill_failed=true deliberately preserves the
// full payload and must not be re-touched here.
func isSizeGovernedResult(result any) bool {
	m, ok := result.(map[string]any)
	if !ok {
		return false
	}
	if truncated, _ := m["truncated"].(bool); truncated {
		return true
	}
	if offloaded, _ := m["offloaded"].(bool); offloaded {
		return true
	}
	if spilled, _ := m["spilled"].(bool); spilled {
		return true
	}
	if spillFailed, _ := m["spill_failed"].(bool); spillFailed {
		return true
	}
	return false
}

// outputLimiterShouldSkip reports whether the AfterTool limiter must not
// rewrite this result. Decorator is the primary size governor; this hook
// only caps undecorated string payloads.
func outputLimiterShouldSkip(toolName string, result any) bool {
	if isSizeGovernedResult(result) {
		return true
	}
	if budgetOverrideForTool(toolName) != nil {
		return true
	}
	if s, ok := result.(string); ok && strings.Contains(s, "[output truncated:") {
		return true
	}
	return false
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
