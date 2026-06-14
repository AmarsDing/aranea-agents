// Package mcpobserve wires MCP session reconnect telemetry into EventBus and Prometheus.
package mcpobserve

import (
	"strings"
	"time"

	mcpdefaults "aranea-agents/internal/mcp"
	mcpconfig "aranea-agents/internal/mcp/config"
)

// NOTE: The framework removed ReconnectObserver/ReconnectEvent callbacks in the
// latest upgrade. Reconnect telemetry (EventBus + Prometheus) is now handled
// internally by the framework. The Observer type and its methods are removed.
// The utility functions below are preserved as they do not depend on removed APIs.

// RecentReconnectWindow is used by the frontend to show a "recent reconnect" chip.
const RecentReconnectWindow = mcpdefaults.RecentReconnectWindow

// DefaultSessionReconnectMax returns the default max reconnect attempts for network transports.
// Uses the canonical mcpconfig.NormalizeTransport so all transport aliases are handled
// consistently (TPM-P1-10).
func DefaultSessionReconnectMax(transport string) int {
	normalized := mcpconfig.NormalizeTransport(transport)
	switch normalized {
	case string(mcpconfig.TransportSSE), string(mcpconfig.TransportStreamable):
		return mcpdefaults.DefaultSessionReconnectMax
	default:
		return 0
	}
}

// EffectiveSessionReconnectMax picks configured max or transport default.
func EffectiveSessionReconnectMax(transport string, configured int) int {
	if configured > 0 {
		return configured
	}
	return DefaultSessionReconnectMax(transport)
}

// IsRecentReconnect reports whether lastReconnectAt (RFC3339) is within RecentReconnectWindow.
func IsRecentReconnect(lastReconnectAt string) bool {
	lastReconnectAt = strings.TrimSpace(lastReconnectAt)
	if lastReconnectAt == "" {
		return false
	}
	t, err := time.Parse(time.RFC3339, lastReconnectAt)
	if err != nil {
		return false
	}
	return time.Since(t) >= 0 && time.Since(t) < RecentReconnectWindow
}
