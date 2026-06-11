package a2a

import (
	"context"
	"strings"
	"time"
)

// Limiter controls per-caller-callee invocation rate.
//
// Implementations MUST be safe for concurrent use. Distributed implementations
// MUST use atomic primitives (e.g. Redis Lua scripts, CAS) to avoid races
// across processes and pods. The Allow method receives the caller's context so
// that distributed implementations can honor request-scoped deadlines/timeouts.
type Limiter interface {
	// Allow reports whether an invocation from `caller` to `callee` is permitted
	// at the current time. A nil error and true means allowed; a nil error and
	// false means rate-limited. A non-nil error means the limiter could not
	// make a decision (e.g. Redis unreachable) — callers should treat that as
	// fail-closed unless they explicitly choose fail-open.
	Allow(ctx context.Context, caller, callee string) (bool, error)
}

// LimiterConfig holds tunable parameters for the A2A sliding-window limiter.
// Both in-memory and Redis-backed implementations share this configuration
// shape so that switching backends is purely a wire-time decision.
type LimiterConfig struct {
	WindowSize time.Duration
	MaxInvokes int
	// KeyPrefix is the Redis key namespace. Ignored by in-memory implementations.
	KeyPrefix string
}

func (c LimiterConfig) applyDefaults() LimiterConfig {
	if c.WindowSize <= 0 {
		c.WindowSize = time.Minute
	}
	if c.MaxInvokes <= 0 {
		c.MaxInvokes = 60
	}
	if strings.TrimSpace(c.KeyPrefix) == "" {
		c.KeyPrefix = "aranea:a2a:invoke:"
	}
	return c
}

// DefaultLimiterConfig returns production defaults: 60 invokes per 60s window.
func DefaultLimiterConfig() LimiterConfig {
	return LimiterConfig{WindowSize: time.Minute, MaxInvokes: 60}.applyDefaults()
}
