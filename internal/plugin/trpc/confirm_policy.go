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
