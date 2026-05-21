// Package metadata merges health and reconnect fields into mcp_server.metadata_json.
package metadata

import (
	"encoding/json"
	"strings"
	"time"
)

const (
	KeyHealthStatus      = "health_status"
	KeyLastHealthAt      = "last_health_at"
	KeyLastErrorMessage  = "last_error_message"
	KeyHealthErrorSince  = "health_error_since"
	KeyLastHealthAlertAt = "last_health_alert_at"
	KeyReconnectCount    = "reconnect_count"
	KeyLastReconnectAt   = "last_reconnect_at"
)

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

// ApplyHealth updates health_* keys and returns the row status to persist ("active" or "error").
func ApplyHealth(m map[string]any, healthStatus string, ok bool, errMsg string, at time.Time) string {
	if m == nil {
		m = map[string]any{}
	}
	m[KeyHealthStatus] = healthStatus
	m[KeyLastHealthAt] = at.UTC().Format(time.RFC3339)
	if ok {
		m[KeyLastErrorMessage] = ""
		delete(m, KeyHealthErrorSince)
		return "active"
	}
	m[KeyLastErrorMessage] = errMsg
	if _, exists := m[KeyHealthErrorSince]; !exists || strings.TrimSpace(metaString(m[KeyHealthErrorSince])) == "" {
		m[KeyHealthErrorSince] = at.UTC().Format(time.RFC3339)
	}
	return "error"
}

func metaString(v any) string {
	s, _ := v.(string)
	return s
}

// ErrorSince returns RFC3339 health_error_since when health_status is error.
func ErrorSince(m map[string]any) string {
	if m == nil {
		return ""
	}
	if strings.TrimSpace(metaString(m[KeyHealthStatus])) != "error" {
		return ""
	}
	return metaString(m[KeyHealthErrorSince])
}

// MarkHealthAlert records that a sustained-health alert was emitted at at.
func MarkHealthAlert(m map[string]any, at time.Time) {
	if m == nil {
		return
	}
	m[KeyLastHealthAlertAt] = at.UTC().Format(time.RFC3339)
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

// ApplyReconnect increments reconnect_count and sets last_reconnect_at.
func ApplyReconnect(m map[string]any, at time.Time) {
	if m == nil {
		m = map[string]any{}
	}
	m[KeyLastReconnectAt] = at.UTC().Format(time.RFC3339)
	switch v := m[KeyReconnectCount].(type) {
	case float64:
		m[KeyReconnectCount] = int(v) + 1
	case int:
		m[KeyReconnectCount] = v + 1
	default:
		m[KeyReconnectCount] = 1
	}
}
