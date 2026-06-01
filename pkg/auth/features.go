package auth

import (
	"os"
	"strings"

	"aranea-agents/pkg/loggateway"
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
	// DEPLOY_ENV not set → refuse bypass; unset env could mean production.
	// SEC-03: never silently allow bypass when the deployment context is unknown.
	if deployEnv == "" {
		loggateway.Global().Warn("auth bypass refused: KRATOS_HTTP_AUTH_DISABLED set but DEPLOY_ENV unset",
			loggateway.StepID("system.auth.bypass_refused"))
		return false
	}
	loggateway.Global().Warn("auth bypass refused: KRATOS_HTTP_AUTH_DISABLED set but DEPLOY_ENV not dev",
		loggateway.StepID("system.auth.bypass_refused"), loggateway.Str("deploy_env", deployEnv))
	return false
}

// WarnIfBypassEnabled logs a prominent startup banner when auth bypass is active.
// Call this once from main() after config is loaded.
func WarnIfBypassEnabled() {
	if HTTPAuthBypassEnabled() {
		loggateway.Global().Warn("AUTH BYPASS ACTIVE: all requests as UserID=1 (admin); DO NOT use in production",
			loggateway.StepID("system.auth.bypass_active"))
	}
}

// DevBypassPrincipal is the injected auth when HTTPAuthBypassEnabled; matches seeded admin id (see internal/data bootstrap).
func DevBypassPrincipal() *Auth {
	return &Auth{UserID: 1, Access: "admin"}
}
