package auth

import "testing"

// TestHTTPAuthBypassCIDecoupling pins F3: CI=true must never enable the auth
// bypass in a named non-dev deployment (doc contract: production always
// refuses); CI only counts when DEPLOY_ENV is unset.
func TestHTTPAuthBypassCIDecoupling(t *testing.T) {
	t.Setenv("KRATOS_HTTP_AUTH_DISABLED", "1")
	SetLogger(nil)

	t.Run("prod env refuses even with CI", func(t *testing.T) {
		t.Setenv("DEPLOY_ENV", "production")
		t.Setenv("CI", "true")
		if HTTPAuthBypassEnabled() {
			t.Fatalf("CI must not smuggle bypass into production")
		}
	})
	t.Run("staging env refuses with CI=1", func(t *testing.T) {
		t.Setenv("DEPLOY_ENV", "staging")
		t.Setenv("CI", "1")
		if HTTPAuthBypassEnabled() {
			t.Fatalf("CI must not smuggle bypass into staging")
		}
	})
	t.Run("unset env honors CI runner", func(t *testing.T) {
		t.Setenv("DEPLOY_ENV", "")
		t.Setenv("CI", "true")
		if !HTTPAuthBypassEnabled() {
			t.Fatalf("CI with unset DEPLOY_ENV should keep bypass (test runner)")
		}
	})
	t.Run("dev env always allowed", func(t *testing.T) {
		t.Setenv("DEPLOY_ENV", "dev")
		t.Setenv("CI", "")
		if !HTTPAuthBypassEnabled() {
			t.Fatalf("dev deployment should allow bypass")
		}
	})
	t.Run("unset env without CI refuses", func(t *testing.T) {
		t.Setenv("DEPLOY_ENV", "")
		t.Setenv("CI", "")
		if HTTPAuthBypassEnabled() {
			t.Fatalf("unset env without CI must refuse")
		}
	})
}

// TestHTTPAuthBypassRequiresRequest pins the master switch: without the
// env request nothing is bypassed regardless of deployment.
func TestHTTPAuthBypassRequiresRequest(t *testing.T) {
	t.Setenv("KRATOS_HTTP_AUTH_DISABLED", "")
	t.Setenv("DEPLOY_ENV", "dev")
	t.Setenv("CI", "true")
	if HTTPAuthBypassEnabled() {
		t.Fatalf("bypass requires KRATOS_HTTP_AUTH_DISABLED")
	}
}
