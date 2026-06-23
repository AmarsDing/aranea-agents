package metadata

import (
	"testing"
	"time"
)

func TestApplyHealth_OKClearsError(t *testing.T) {
	m := Parse(`{"last_error_message":"old","health_error_since":"2026-05-21T00:00:00Z"}`)
	updated, st := ApplyHealth(m, "ok", true, "", time.Date(2026, 5, 21, 0, 0, 0, 0, time.UTC))
	if st != "active" {
		t.Fatalf("status=%q", st)
	}
	if updated[KeyLastErrorMessage] != "" {
		t.Fatalf("last_error_message=%v", updated[KeyLastErrorMessage])
	}
	if _, ok := updated[KeyHealthErrorSince]; ok {
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

// TestApplyHealth_AuthRequiredPreservesErrorSince verifies the S1 fix:
// when ok=true but healthStatus="auth_required", ApplyHealth must preserve
// (or initialize) health_error_since so the alert debounce window works.
// The persisted row status remains "active" because the server is up.
func TestApplyHealth_AuthRequiredPreservesErrorSince(t *testing.T) {
	at := time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC)

	// Case 1: fresh auth_required (no prior error_since) — should initialize it.
	m1 := Parse(`{}`)
	updated1, st1 := ApplyHealth(m1, "auth_required", true, "needs OAuth", at)
	if st1 != "active" {
		t.Fatalf("status=%q, want active", st1)
	}
	since1, ok1 := updated1[KeyHealthErrorSince]
	if !ok1 {
		t.Fatal("expected health_error_since to be initialized for auth_required")
	}
	if since1 != at.Format(time.RFC3339) {
		t.Fatalf("health_error_since=%v, want %v", since1, at.Format(time.RFC3339))
	}
	if updated1[KeyLastErrorMessage] != "needs OAuth" {
		t.Fatalf("last_error_message=%v", updated1[KeyLastErrorMessage])
	}

	// Case 2: auth_required after prior error — should preserve original error_since.
	prior := at.Add(-10 * time.Minute).Format(time.RFC3339)
	m2 := Parse(`{"health_error_since":"` + prior + `"}`)
	updated2, st2 := ApplyHealth(m2, "auth_required", true, "needs OAuth", at)
	if st2 != "active" {
		t.Fatalf("status=%q, want active", st2)
	}
	if updated2[KeyHealthErrorSince] != prior {
		t.Fatalf("health_error_since=%v, want preserved %v", updated2[KeyHealthErrorSince], prior)
	}
}

// TestErrorSince_RecognizesAuthRequired verifies the S1 fix: ErrorSince
// returns the timestamp for both "error" and "auth_required" statuses.
func TestErrorSince_RecognizesAuthRequired(t *testing.T) {
	ts := "2026-05-21T12:00:00Z"

	errorMeta := map[string]any{KeyHealthStatus: "error", KeyHealthErrorSince: ts}
	if got := ErrorSince(errorMeta); got != ts {
		t.Fatalf("error status: got %q, want %q", got, ts)
	}

	authMeta := map[string]any{KeyHealthStatus: "auth_required", KeyHealthErrorSince: ts}
	if got := ErrorSince(authMeta); got != ts {
		t.Fatalf("auth_required status: got %q, want %q", got, ts)
	}

	// Healthy status returns empty.
	okMeta := map[string]any{KeyHealthStatus: "ok", KeyHealthErrorSince: ts}
	if got := ErrorSince(okMeta); got != "" {
		t.Fatalf("ok status: got %q, want empty", got)
	}
}

// TestShouldEmitHealthAlert_AuthRequired verifies the end-to-end S1 fix:
// auth_required state with a sufficiently old health_error_since triggers
// alert emission, and is debounced by last_health_alert_at.
func TestShouldEmitHealthAlert_AuthRequired(t *testing.T) {
	now := time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC)
	m := map[string]any{
		KeyHealthStatus:     "auth_required",
		KeyHealthErrorSince: now.Add(-10 * time.Minute).Format(time.RFC3339),
	}
	if !ShouldEmitHealthAlert(m, now, 5*time.Minute) {
		t.Fatal("expected alert for sustained auth_required")
	}
	m[KeyLastHealthAlertAt] = now.Add(-2 * time.Minute).Format(time.RFC3339)
	if ShouldEmitHealthAlert(m, now, 5*time.Minute) {
		t.Fatal("expected debounced for auth_required")
	}
}
