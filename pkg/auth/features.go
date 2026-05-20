package auth

import (
	"fmt"
	"os"
	"strings"
)

// HTTPAuthBypassEnabled skips cookie/JWT checks on HTTP when true (KRATOS_HTTP_AUTH_DISABLED).
// EP-SEC-02: Only permitted when DEPLOY_ENV is "dev" or "development", or when running
// under a recognized CI environment. Never enable in production.
func HTTPAuthBypassEnabled() bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv("KRATOS_HTTP_AUTH_DISABLED")))
	if v != "1" && v != "true" && v != "yes" {
		return false
	}
	deployEnv := strings.TrimSpace(strings.ToLower(os.Getenv("DEPLOY_ENV")))
	ci := strings.TrimSpace(os.Getenv("CI"))
	if deployEnv == "dev" || deployEnv == "development" || deployEnv == "test" || ci == "true" || ci == "1" {
		return true
	}
	// DEPLOY_ENV not set → allow bypass but emit a warning so operators notice.
	if deployEnv == "" {
		fmt.Fprintln(os.Stderr, "[flow][system] system.auth.bypass_warn: KRATOS_HTTP_AUTH_DISABLED set but DEPLOY_ENV unset; bypass active")
		_ = os.Stderr.Sync()
		return true
	}
	// Any other DEPLOY_ENV (e.g. "production", "staging") → refuse bypass.
	fmt.Fprintf(os.Stderr, "[flow][system] system.auth.bypass_refused: KRATOS_HTTP_AUTH_DISABLED set but DEPLOY_ENV=%s\n", deployEnv)
	_ = os.Stderr.Sync()
	return false
}

// WarnIfBypassEnabled logs a prominent startup banner when auth bypass is active.
// Call this once from main() after config is loaded.
func WarnIfBypassEnabled() {
	if HTTPAuthBypassEnabled() {
		fmt.Fprintln(os.Stderr, "[flow][system] system.auth.bypass_active: AUTH BYPASS ACTIVE — all requests as UserID=1 (admin); DO NOT use in production")
		_ = os.Stderr.Sync()
	}
}

// DevBypassPrincipal is the injected auth when HTTPAuthBypassEnabled; matches seeded admin id (see internal/data bootstrap).
func DevBypassPrincipal() *Auth {
	return &Auth{UserID: 1, Access: "admin"}
}
