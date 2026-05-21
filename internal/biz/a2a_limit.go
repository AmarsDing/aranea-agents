package biz

import (
	"sync"
	"time"
)

// A2AInvokeLimiter provides per-caller/callee rate limiting for A2A Invoke (Phase 4).
type A2AInvokeLimiter struct {
	mu      sync.Mutex
	window  time.Duration
	max     int
	buckets map[string][]time.Time
}

func NewA2AInvokeLimiter(maxPerWindow int, window time.Duration) *A2AInvokeLimiter {
	if maxPerWindow <= 0 {
		maxPerWindow = 60
	}
	if window <= 0 {
		window = time.Minute
	}
	return &A2AInvokeLimiter{
		window:  window,
		max:     maxPerWindow,
		buckets: make(map[string][]time.Time),
	}
}

func (l *A2AInvokeLimiter) Allow(caller, callee string) bool {
	if l == nil {
		return true
	}
	key := caller + "->" + callee
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()
	cutoff := now.Add(-l.window)
	entries := l.buckets[key]
	kept := entries[:0]
	for _, ts := range entries {
		if ts.After(cutoff) {
			kept = append(kept, ts)
		}
	}
	if len(kept) >= l.max {
		l.buckets[key] = kept
		return false
	}
	l.buckets[key] = append(kept, now)
	return true
}

var defaultA2ALimiter = NewA2AInvokeLimiter(60, time.Minute)

// DefaultA2AInvokeLimiter returns the process-wide A2A invoke limiter.
func DefaultA2AInvokeLimiter() *A2AInvokeLimiter { return defaultA2ALimiter }
