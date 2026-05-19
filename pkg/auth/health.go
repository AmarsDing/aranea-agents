package auth

import (
	"os"
	"strings"
)

// HealthAuth is returned by GET /healthz for startup diagnostics.
type HealthAuth struct {
	Status     string `json:"status"`
	AuthMode   string `json:"auth_mode"`
	CookieName string `json:"cookie_name"`
	WSPath     string `json:"ws_path"`
	DeployEnv  string `json:"deploy_env"`
}

// HealthAuthInfo builds the auth slice of the health check response.
func HealthAuthInfo() HealthAuth {
	mode := "jwt"
	if HTTPAuthBypassEnabled() {
		mode = "bypass"
	}
	return HealthAuth{
		Status:     "ok",
		AuthMode:   mode,
		CookieName: cookieName,
		WSPath:     "/v1/ws",
		DeployEnv:  strings.TrimSpace(os.Getenv("DEPLOY_ENV")),
	}
}
