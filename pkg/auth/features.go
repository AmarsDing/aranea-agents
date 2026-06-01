package auth

import (
	"os"
	"strings"

	"aranea-agents/pkg/loggateway"
)

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
	if deployEnv == "" {
		loggateway.Global().Warn("auth bypass refused: KRATOS_HTTP_AUTH_DISABLED set but DEPLOY_ENV unset",
			loggateway.StepID("auth.bypass_refused"))
		return false
	}
	loggateway.Global().Warn("auth bypass refused: KRATOS_HTTP_AUTH_DISABLED set but DEPLOY_ENV not dev",
		loggateway.StepID("auth.bypass_refused"), loggateway.Str("deploy_env", deployEnv))
	return false
}

func WarnIfBypassEnabled() {
	if HTTPAuthBypassEnabled() {
		loggateway.Global().Warn("AUTH BYPASS ACTIVE: all requests as UserID=1 (admin); DO NOT use in production",
			loggateway.StepID("auth.bypass_active"))
	}
}

func DevBypassPrincipal() *Auth {
	return &Auth{UserID: 1, Access: "admin"}
}
