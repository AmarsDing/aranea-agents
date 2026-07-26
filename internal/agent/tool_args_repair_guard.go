package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"aranea-agents/internal/agent/callbacks"
	"aranea-agents/pkg/loggateway"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// newToolArgsRepairBeforeHook repairs malformed JSON tool arguments emitted
// by the model before any downstream guard or the tool itself parses them.
//
// Root cause of the 2026-07-25 22:32–22:36 orchestration_failed incident:
// deepseek-v4-flash emitted syntactically broken arguments for set_deliverable
// (trailing extra '}', trailing ',', raw newlines inside string literals).
// Three retry_reflect rounds all failed, no deliverable was written, and the
// deliverable gate flipped both teams from completed to failed.
//
// Repair strategies, in order (first success wins):
//  1. Decode the first complete JSON value and discard trailing garbage —
//     fixes "invalid character '}'/',' after top-level value".
//  2. Escape raw control characters (\n, \r, \t, …) inside string literals —
//     fixes "invalid character '\n' in string literal".
//  3. Escape + first-value decode combined.
//
// Only repairs whose result is a complete, valid JSON OBJECT are accepted
// (tool arguments are objects by contract). Unrepairable input passes through
// unchanged so retry_reflect can hand the original error back to the model.
//
// Priority 1: must run before the todo-args guard (2) and the system-keys
// guard (3), both of which no-op on non-JSON input.
func newToolArgsRepairBeforeHook(lg loggateway.Logger) callbacks.BeforeToolHook {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	logger := lg
	return callbacks.NewBeforeToolHook(1, func(ctx context.Context, args *trpctool.BeforeToolArgs) (*trpctool.BeforeToolResult, error) {
		if args == nil || len(args.Arguments) == 0 {
			return &trpctool.BeforeToolResult{Context: ctx}, nil
		}
		repaired, changed := repairToolArguments(args.Arguments)
		if !changed {
			return &trpctool.BeforeToolResult{Context: ctx}, nil
		}
		// Never log argument content (may contain secrets); sizes + tool name
		// are enough to correlate with the audit trail.
		logger.Warn("tool args guard: repaired malformed JSON arguments",
			loggateway.StepID("tool.args_repair"),
			loggateway.Str("tool", args.ToolName),
			loggateway.Str("orig_len", fmt.Sprint(len(args.Arguments))),
			loggateway.Str("repaired_len", fmt.Sprint(len(repaired))),
		)
		args.Arguments = repaired
		return &trpctool.BeforeToolResult{Context: ctx}, nil
	})
}

// repairToolArguments returns (repaired, true) when the input was malformed
// but a valid JSON object could be recovered; (nil, false) when the input is
// already valid or cannot be repaired safely.
func repairToolArguments(raw []byte) ([]byte, bool) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || json.Valid(trimmed) {
		return nil, false
	}
	// Stage 1: trailing garbage after the first complete value.
	if v, ok := decodeFirstJSONObject(trimmed); ok {
		return v, true
	}
	// Stage 2: raw control characters inside string literals.
	escaped := escapeControlCharsInStrings(trimmed)
	if json.Valid(escaped) {
		return escaped, true
	}
	// Stage 3: escaped + trailing garbage.
	if v, ok := decodeFirstJSONObject(escaped); ok {
		return v, true
	}
	return nil, false
}

// decodeFirstJSONObject decodes the first complete JSON value from b and
// re-marshals it, discarding any trailing bytes. Only JSON objects are
// accepted; UseNumber preserves large integers through the round trip.
func decodeFirstJSONObject(b []byte) ([]byte, bool) {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	var obj map[string]any
	if err := dec.Decode(&obj); err != nil {
		return nil, false
	}
	out, err := json.Marshal(obj)
	if err != nil {
		return nil, false
	}
	return out, true
}

// escapeControlCharsInStrings replaces raw control characters (< 0x20) that
// appear inside JSON string literals with their \u00XX escapes. Bytes outside
// strings and already-escaped sequences pass through untouched.
func escapeControlCharsInStrings(b []byte) []byte {
	var out bytes.Buffer
	out.Grow(len(b) + 16)
	inStr, esc := false, false
	for _, ch := range b {
		if esc {
			esc = false
			out.WriteByte(ch)
			continue
		}
		if inStr {
			switch {
			case ch == '\\':
				esc = true
				out.WriteByte(ch)
			case ch == '"':
				inStr = false
				out.WriteByte(ch)
			case ch < 0x20:
				fmt.Fprintf(&out, `\u%04x`, ch)
			default:
				out.WriteByte(ch)
			}
			continue
		}
		if ch == '"' {
			inStr = true
		}
		out.WriteByte(ch)
	}
	return out.Bytes()
}
