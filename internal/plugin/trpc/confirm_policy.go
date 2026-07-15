package plugintrpc

import (
	"context"
	"encoding/json"
	"strings"
)

// ConfirmationGuardConfig is the product configuration for confirmation_guard.
type ConfirmationGuardConfig struct {
	ConfirmTools    []string `json:"confirm_tools"`
	ConfirmPatterns []string `json:"confirm_patterns"`
	TimeoutSeconds  int      `json:"timeout_seconds"`
	DefaultAction   string   `json:"default_action"`
}

// MatchConfirmationGuard reports whether a tool call matches plugin confirmation rules.
func MatchConfirmationGuard(cfg ConfirmationGuardConfig, toolName string, args []byte) bool {
	if toolInList(toolName, cfg.ConfirmTools) {
		return true
	}
	if len(cfg.ConfirmPatterns) == 0 {
		return false
	}
	raw := string(args)
	if raw == "" {
		return false
	}
	var m map[string]any
	if err := json.Unmarshal(args, &m); err != nil {
		return containsAny(raw, cfg.ConfirmPatterns)
	}
	b, _ := json.Marshal(m)
	return containsAny(string(b), cfg.ConfirmPatterns)
}

// ConfirmationDefaultAllow reports whether unmatched human channel falls back to allow.
func ConfirmationDefaultAllow(cfg ConfirmationGuardConfig) bool {
	return strings.EqualFold(strings.TrimSpace(cfg.DefaultAction), "allow")
}

// CatalogConfirmChecker reports catalog requires_confirmation for an agent tool.
type CatalogConfirmChecker func(ctx context.Context, agentID, toolName string) bool

// --- P1-10: unified confirmation path ---
//
// The product callback (internal/agent/tool_confirmation.go) and the
// confirmation_guard Runner Plugin (confirmation_guard.go) both check
// MatchConfirmationGuard. Without coordination, the plugin — which runs
// after Chain callbacks in mergeToolCallbacks — would re-block a tool the
// user just approved via the product callback's confirm card.
//
// WithToolConfirmHandled lets the product callback mark the context after
// user approval; the plugin checks ToolConfirmHandled at the start of
// beforeTool and skips its own check when the product path has already
// handled confirmation. This collapses two independent confirmation
// decisions into a single state machine: product callback (with UI) is
// authoritative; plugin acts only as a fallback when the product callback
// did not run (e.g., callback chain misconfigured or skipped).
type toolConfirmHandledKey struct{}

// WithToolConfirmHandled returns a derived context tagged with the
// "confirmation handled" flag. The product callback calls this after the
// user approves a tool confirmation card.
func WithToolConfirmHandled(ctx context.Context) context.Context {
	return context.WithValue(ctx, toolConfirmHandledKey{}, true)
}

// ToolConfirmHandled reports whether the context already carries a
// "confirmation handled" marker. The confirmation_guard plugin calls
// this at the top of beforeTool to skip its redundant check.
func ToolConfirmHandled(ctx context.Context) bool {
	v, _ := ctx.Value(toolConfirmHandledKey{}).(bool)
	return v
}
