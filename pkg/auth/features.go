package auth

import (
	"os"
	"strings"
	"sync"

	"aranea-agents/pkg/loggateway"
)

var (
	authLogger   loggateway.Logger
	authLoggerMu sync.RWMutex
)

// SetLogger injects the loggateway.Logger used by auth package functions.
// Must be called once during application startup.
func SetLogger(lg loggateway.Logger) {
	authLoggerMu.Lock()
	defer authLoggerMu.Unlock()
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	authLogger = lg
}

func getLogger() loggateway.Logger {
	authLoggerMu.RLock()
	defer authLoggerMu.RUnlock()
	if authLogger == nil {
		return loggateway.NewNoop()
	}
	return authLogger
}

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
	lg := getLogger()
	if deployEnv == "" {
		lg.Warn("auth bypass refused: KRATOS_HTTP_AUTH_DISABLED set but DEPLOY_ENV unset",
			loggateway.StepID("auth.bypass_refused"))
		return false
	}
	lg.Warn("auth bypass refused: KRATOS_HTTP_AUTH_DISABLED set but DEPLOY_ENV not dev",
		loggateway.StepID("auth.bypass_refused"), loggateway.Str("deploy_env", deployEnv))
	return false
}

func WarnIfBypassEnabled() {
	if HTTPAuthBypassEnabled() {
		lg := getLogger()
		lg.Warn("AUTH BYPASS ACTIVE: all requests as UserID=1 (admin); DO NOT use in production",
			loggateway.StepID("auth.bypass_active"))
	}
}

func DevBypassPrincipal() *Auth {
	return &Auth{UserID: 1, Access: "admin"}
}
