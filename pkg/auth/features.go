package auth

import (
	"os"
	"strings"
)

// HTTPAuthBypassEnabled skips cookie/JWT checks on HTTP when true (KRATOS_HTTP_AUTH_DISABLED).
// For local SPA development only — never enable in production.
func HTTPAuthBypassEnabled() bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv("KRATOS_HTTP_AUTH_DISABLED")))
	return v == "1" || v == "true" || v == "yes"
}

// DevBypassPrincipal is the injected auth when HTTPAuthBypassEnabled; matches seeded admin id (see internal/data bootstrap).
func DevBypassPrincipal() *Auth {
	return &Auth{UserID: 1, Access: "admin"}
}
