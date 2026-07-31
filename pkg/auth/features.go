package auth

import (
	"context"
	"os"
	"strings"
	"sync"
	"time"

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

// BypassRequested reports whether KRATOS_HTTP_AUTH_DISABLED requests a bypass,
// regardless of the DEPLOY_ENV/CI gating in HTTPAuthBypassEnabled. Used by the
// startup orchestration layer to distinguish "bypass active" from "bypass
// refused" for one-shot flow-log emission (pkg cannot import internal/event).
func BypassRequested() bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv("KRATOS_HTTP_AUTH_DISABLED")))
	return v == "1" || v == "true" || v == "yes"
}

func HTTPAuthBypassEnabled() bool {
	if !BypassRequested() {
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
	return &Auth{UserID: 1, Access: "admin", WorkspaceID: DefaultWorkspaceID}
}

// --- Flow-log hooks (pkg cannot import internal/event) ---

var (
	onGRPCUnauthenticated   func(ctx context.Context)
	onGRPCUnauthenticatedMu sync.RWMutex
)

// SetOnGRPCUnauthenticated registers the hook fired when a gRPC request
// arrives without credentials. Called once at startup by the internal server
// layer (internal/server/grpc.go), which bridges to internal/event flow logs.
func SetOnGRPCUnauthenticated(fn func(ctx context.Context)) {
	onGRPCUnauthenticatedMu.Lock()
	defer onGRPCUnauthenticatedMu.Unlock()
	onGRPCUnauthenticated = fn
}

func emitGRPCUnauthenticated(ctx context.Context) {
	onGRPCUnauthenticatedMu.RLock()
	fn := onGRPCUnauthenticated
	onGRPCUnauthenticatedMu.RUnlock()
	if fn != nil {
		fn(ctx)
	}
}

// --- Login-failure log throttle (K2: 防爆破刷屏) ---

// logThrottle is a minimal time-window throttle for hot Warn paths: the first
// occurrence within each window is logged (with the count of suppressed
// occurrences since the last log), the rest are counted and dropped.
type logThrottle struct {
	mu         sync.Mutex
	interval   time.Duration
	last       time.Time
	suppressed int
}

func newLogThrottle(interval time.Duration) *logThrottle {
	return &logThrottle{interval: interval}
}

// allow reports whether the current occurrence may be logged, and how many
// occurrences were suppressed since the previous logged one.
func (t *logThrottle) allow() (bool, int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := time.Now()
	if now.Sub(t.last) < t.interval {
		t.suppressed++
		return false, 0
	}
	sup := t.suppressed
	t.suppressed = 0
	t.last = now
	return true, sup
}

// tokenInvalidThrottle throttles "token parse failed" Warns (public-facing
// endpoints are brute-force targets): at most one log per 10s window.
var tokenInvalidThrottle = newLogThrottle(10 * time.Second)
