package metadata

import (
	"testing"
	"time"
)

func TestApplyHealth_OKClearsError(t *testing.T) {
	m := Parse(`{"last_error_message":"old","health_error_since":"2026-05-21T00:00:00Z"}`)
	st := ApplyHealth(m, "ok", true, "", time.Date(2026, 5, 21, 0, 0, 0, 0, time.UTC))
	if st != "active" {
		t.Fatalf("status=%q", st)
	}
	if m[KeyLastErrorMessage] != "" {
		t.Fatalf("last_error_message=%v", m[KeyLastErrorMessage])
	}
	if _, ok := m[KeyHealthErrorSince]; ok {
		t.Fatal("expected health_error_since cleared")
	}
}

func TestShouldEmitHealthAlert(t *testing.T) {
	now := time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC)
	m := map[string]any{
		KeyHealthStatus:     "error",
		KeyHealthErrorSince: now.Add(-10 * time.Minute).Format(time.RFC3339),
	}
	if !ShouldEmitHealthAlert(m, now, 5*time.Minute) {
		t.Fatal("expected alert")
	}
	m[KeyLastHealthAlertAt] = now.Add(-2 * time.Minute).Format(time.RFC3339)
	if ShouldEmitHealthAlert(m, now, 5*time.Minute) {
		t.Fatal("expected debounced")
	}
}
