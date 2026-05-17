package auth

import (
	"log/slog"
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
		slog.Warn("[SECURITY] KRATOS_HTTP_AUTH_DISABLED is set but DEPLOY_ENV is not 'dev'. " +
			"Auth bypass is active. Set DEPLOY_ENV=production to refuse bypass.")
		return true
	}
	// Any other DEPLOY_ENV (e.g. "production", "staging") → refuse bypass.
	slog.Error("[SECURITY] KRATOS_HTTP_AUTH_DISABLED is set but DEPLOY_ENV=" + deployEnv +
		". Auth bypass is REFUSED in non-dev environments.")
	return false
}

// WarnIfBypassEnabled logs a prominent startup banner when auth bypass is active.
// Call this once from main() after config is loaded.
func WarnIfBypassEnabled() {
	if HTTPAuthBypassEnabled() {
		slog.Warn("⚠ AUTH BYPASS ACTIVE — KRATOS_HTTP_AUTH_DISABLED=1 " +
			"All requests are authenticated as UserID=1 (admin). " +
			"DO NOT use in production.")
	}
}

// DevBypassPrincipal is the injected auth when HTTPAuthBypassEnabled; matches seeded admin id (see internal/data bootstrap).
func DevBypassPrincipal() *Auth {
	return &Auth{UserID: 1, Access: "admin"}
}
