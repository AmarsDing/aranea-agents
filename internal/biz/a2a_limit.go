package biz

import (
	"sync"
	"time"
)

// A2ALimiter controls per-caller-callee invocation rate.
// TODO(debt): DEV-08 — Replace in-memory implementation with Redis-backed
// distributed limiter for multi-pod deployments.
type A2ALimiter interface {
	Allow(caller, callee string) bool
}

// A2ALimiterConfig holds configurable parameters for the in-memory sliding
// window limiter. Future Redis-backed implementations may define their own
// config structs.
type A2ALimiterConfig struct {
	WindowSize time.Duration
	MaxInvokes int
}

// DefaultA2ALimiterConfig returns the current production defaults.
func DefaultA2ALimiterConfig() A2ALimiterConfig {
	return A2ALimiterConfig{
		WindowSize: time.Minute,
		MaxInvokes: 60,
	}
}

// slidingWindowLimiter is an in-memory per-caller/callee rate limiter using a
// sliding window algorithm. It implements A2ALimiter.
type slidingWindowLimiter struct {
	mu      sync.Mutex
	window  time.Duration
	max     int
	buckets map[string][]time.Time
}

// NewSlidingWindowLimiter creates an in-memory A2ALimiter from cfg.
func NewSlidingWindowLimiter(cfg A2ALimiterConfig) *slidingWindowLimiter {
	max := cfg.MaxInvokes
	if max <= 0 {
		max = 60
	}
	window := cfg.WindowSize
	if window <= 0 {
		window = time.Minute
	}
	return &slidingWindowLimiter{
		window:  window,
		max:     max,
		buckets: make(map[string][]time.Time),
	}
}

func (l *slidingWindowLimiter) Allow(caller, callee string) bool {
	if l == nil {
		return true
	}
	key := caller + "->" + callee
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()
	cutoff := now.Add(-l.window)
	entries := l.buckets[key]
	kept := make([]time.Time, 0, len(entries))
	for _, ts := range entries {
		if ts.After(cutoff) {
			kept = append(kept, ts)
		}
	}
	// Clean up stale buckets to prevent unbounded memory growth.
	if len(kept) == 0 {
		delete(l.buckets, key)
	}
	if len(kept) >= l.max {
		l.buckets[key] = kept
		return false
	}
	l.buckets[key] = append(kept, now)
	return true
}

// A2AInvokeLimiter is kept for backward compatibility; it wraps
// *slidingWindowLimiter and implements A2ALimiter.
type A2AInvokeLimiter = slidingWindowLimiter

// NewA2AInvokeLimiter is kept for backward compatibility.
func NewA2AInvokeLimiter(maxPerWindow int, window time.Duration) *A2AInvokeLimiter {
	return NewSlidingWindowLimiter(A2ALimiterConfig{
		MaxInvokes: maxPerWindow,
		WindowSize: window,
	})
}

var defaultA2ALimiter = NewSlidingWindowLimiter(DefaultA2ALimiterConfig())

// DefaultA2AInvokeLimiter returns the process-wide A2A invoke limiter.
func DefaultA2AInvokeLimiter() *A2AInvokeLimiter { return defaultA2ALimiter }
