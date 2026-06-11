package a2a

import (
	"context"
	"sync"
	"time"
)

// memorySlidingWindowLimiter is an in-memory per-caller/callee rate limiter
// using a sliding window algorithm. It implements Limiter.
//
// This implementation is process-local. It is suitable for development and
// single-pod deployments. For multi-pod deployments use
// NewRedisSlidingWindowLimiter (constructed via NewLimiter when a Redis
// client is available).
type memorySlidingWindowLimiter struct {
	mu      sync.Mutex
	window  time.Duration
	max     int
	buckets map[string][]time.Time
}

// NewMemorySlidingWindowLimiter creates an in-memory Limiter from cfg.
func NewMemorySlidingWindowLimiter(cfg LimiterConfig) *memorySlidingWindowLimiter {
	cfg = cfg.applyDefaults()
	return &memorySlidingWindowLimiter{
		window:  cfg.WindowSize,
		max:     cfg.MaxInvokes,
		buckets: make(map[string][]time.Time),
	}
}

func (l *memorySlidingWindowLimiter) Allow(_ context.Context, caller, callee string) (bool, error) {
	if l == nil {
		return true, nil
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
		return false, nil
	}
	l.buckets[key] = append(kept, now)
	return true, nil
}

// MemorySlidingWindowLimiter is the historical alias kept for code that
// references the old type name. New code should depend on the Limiter
// interface and let NewLimiter pick the implementation.
type MemorySlidingWindowLimiter = memorySlidingWindowLimiter

// NewInvokeLimiter creates an in-memory limiter. Kept for backward
// compatibility; production wiring should use NewLimiter.
func NewInvokeLimiter(maxPerWindow int, window time.Duration) *MemorySlidingWindowLimiter {
	return NewMemorySlidingWindowLimiter(LimiterConfig{
		MaxInvokes: maxPerWindow,
		WindowSize: window,
	})
}
