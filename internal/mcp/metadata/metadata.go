// Package metadata merges health and reconnect fields into mcp_server.metadata_json.
package metadata

import (
	"encoding/json"
	"strings"
	"time"

	"aranea-agents/internal/mcp/lifecycle"
)

const (
	KeyHealthStatus      = "health_status"
	KeyLastHealthAt      = "last_health_at"
	KeyLastErrorMessage  = "last_error_message"
	KeyHealthErrorSince  = "health_error_since"
	KeyLastHealthAlertAt = "last_health_alert_at"
	KeyReconnectCount    = "reconnect_count"
	KeyLastReconnectAt   = "last_reconnect_at"
	// Tool discovery keys (P2): written by real MCP handshake (initialize +
	// tools/list), orthogonal to health_* — discovery failure never flips
	// health_status.
	KeyToolCount          = "tool_count"
	KeyToolNames          = "tool_names"
	KeyToolsDiscoveredAt  = "tools_discovered_at"
	KeyToolsErrorMessage  = "tools_error_message"
)

// maxStoredToolNames caps tool_names persisted into metadata_json so a
// server exposing hundreds of tools cannot bloat the row.
const maxStoredToolNames = 50

// Parse unmarshals metadata_json; invalid JSON yields an empty map.
func Parse(raw string) map[string]any {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		raw = "{}"
	}
	var m map[string]any
	if json.Unmarshal([]byte(raw), &m) != nil {
		return map[string]any{}
	}
	if m == nil {
		return map[string]any{}
	}
	return m
}

// Marshal encodes metadata for persistence.
func Marshal(m map[string]any) (string, error) {
	if m == nil {
		m = map[string]any{}
	}
	b, err := json.Marshal(m)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// ApplyHealth returns a new map with health_* keys updated and the row status to persist ("active" or "error").
// The input map is not modified.
//
// Special case: when ok=true but healthStatus=="auth_required", the server is
// network-reachable but requires credentials the probe does not inject. This is
// a degraded state worth alerting on, so health_error_since is preserved (or
// initialized) to feed the alert debounce window — same as the error branch.
// The persisted row status remains "active" because the server itself is up.
func ApplyHealth(m map[string]any, healthStatus string, ok bool, errMsg string, at time.Time) (map[string]any, string) {
	out := cloneMap(m)
	prev := lifecycle.Normalize(metaString(out[KeyHealthStatus]))
	ev := lifecycle.EventFromProbeStatus(healthStatus)
	if ok && healthStatus != "auth_required" && healthStatus != "degraded" {
		ev = lifecycle.EventProbeOK
	}
	next, err := lifecycle.Transition(prev, ev)
	if err != nil {
		next = lifecycle.Normalize(healthStatus)
	}
	out[KeyHealthStatus] = string(next)
	out[KeyLastHealthAt] = at.UTC().Format(time.RFC3339)
	if next == lifecycle.StateOK {
		out[KeyLastErrorMessage] = ""
		delete(out, KeyHealthErrorSince)
		return out, "active"
	}
	out[KeyLastErrorMessage] = errMsg
	if _, exists := out[KeyHealthErrorSince]; !exists || strings.TrimSpace(metaString(out[KeyHealthErrorSince])) == "" {
		out[KeyHealthErrorSince] = at.UTC().Format(time.RFC3339)
	}
	if next == lifecycle.StateAuthRequired || next == lifecycle.StateDegraded {
		// auth_required / degraded: server may still be up — persist as "active".
		return out, "active"
	}
	return out, "error"
}

// MarkHealthAlert returns a new map with last_health_alert_at set.
// The input map is not modified.
func MarkHealthAlert(m map[string]any, at time.Time) map[string]any {
	out := cloneMap(m)
	out[KeyLastHealthAlertAt] = at.UTC().Format(time.RFC3339)
	return out
}

// ApplyReconnect returns a new map with reconnect_count incremented and last_reconnect_at set.
// The input map is not modified.
func ApplyReconnect(m map[string]any, at time.Time) map[string]any {
	out := cloneMap(m)
	out[KeyLastReconnectAt] = at.UTC().Format(time.RFC3339)
	switch v := out[KeyReconnectCount].(type) {
	case float64:
		out[KeyReconnectCount] = v + 1
	default:
		out[KeyReconnectCount] = float64(1)
	}
	return out
}

// ApplyToolDiscovery returns a new map with a successful tool discovery merged:
// tool_count (the server's full exposed count), tool_names (capped at
// maxStoredToolNames — callers may pass a pre-capped slice while count stays
// exact), tools_discovered_at; any previous tools_error_message is cleared.
// The input map is not modified. tool_count is stored as float64 to match
// JSON unmarshal semantics (Parse).
func ApplyToolDiscovery(m map[string]any, count int, names []string, at time.Time) map[string]any {
	out := cloneMap(m)
	stored := make([]any, 0, minInt(len(names), maxStoredToolNames))
	for i, n := range names {
		if i >= maxStoredToolNames {
			break
		}
		stored = append(stored, n)
	}
	out[KeyToolCount] = float64(count)
	out[KeyToolNames] = stored
	out[KeyToolsDiscoveredAt] = at.UTC().Format(time.RFC3339)
	delete(out, KeyToolsErrorMessage)
	return out
}

// ApplyToolDiscoveryError returns a new map recording a failed discovery
// attempt: tools_error_message + tools_discovered_at (the attempt timestamp
// feeds the refresh cadence so a broken server is not re-probed every health
// tick). Previously discovered tool_count/tool_names are preserved — stale
// data plus a visible error is friendlier than blanking the column.
func ApplyToolDiscoveryError(m map[string]any, errMsg string, at time.Time) map[string]any {
	out := cloneMap(m)
	out[KeyToolsErrorMessage] = errMsg
	out[KeyToolsDiscoveredAt] = at.UTC().Format(time.RFC3339)
	return out
}

// ToolsDiscoveryStale reports whether the stored discovery is missing or older
// than after. Servers never discovered (empty timestamp) are stale.
func ToolsDiscoveryStale(m map[string]any, now time.Time, after time.Duration) bool {
	raw := strings.TrimSpace(metaString(m[KeyToolsDiscoveredAt]))
	if raw == "" {
		return true
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return true
	}
	return now.Sub(t) >= after
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func metaString(v any) string {
	s, _ := v.(string)
	return s
}

// ErrorSince returns RFC3339 health_error_since when health_status indicates
// a degraded state ("error" or "auth_required"). Both states feed the alert
// debounce window in ShouldEmitHealthAlert.
func ErrorSince(m map[string]any) string {
	if m == nil {
		return ""
	}
	status := strings.TrimSpace(metaString(m[KeyHealthStatus]))
	if status != "error" && status != "auth_required" {
		return ""
	}
	return metaString(m[KeyHealthErrorSince])
}

// ShouldEmitHealthAlert returns true when error persisted longer than after and no recent alert.
func ShouldEmitHealthAlert(m map[string]any, now time.Time, after time.Duration) bool {
	since := ErrorSince(m)
	if since == "" {
		return false
	}
	t, err := time.Parse(time.RFC3339, since)
	if err != nil {
		return false
	}
	if now.Sub(t) < after {
		return false
	}
	last := metaString(m[KeyLastHealthAlertAt])
	if last == "" {
		return true
	}
	lt, err := time.Parse(time.RFC3339, last)
	if err != nil {
		return true
	}
	return now.Sub(lt) >= after
}

// cloneMap returns a shallow copy of m.
func cloneMap(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
