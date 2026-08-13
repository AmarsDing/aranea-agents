package loggateway

import (
	"sync"
	"time"
)

// Throttle is a time-window log throttle for hot Warn/Error paths: the first
// occurrence within each window is allowed (and reports how many occurrences
// were suppressed since the last allowed one), the rest are counted and
// dropped. Use it wherever a persistent downstream failure (DB down, disk
// full) would otherwise turn every operation into a log line and flood the
// pipeline.
//
// A nil *Throttle permits everything (Allow returns true, 0), so optional
// throttles need no nil checks at call sites.
type Throttle struct {
	mu         sync.Mutex
	interval   time.Duration
	last       time.Time
	suppressed int
}

// NewThrottle returns a Throttle with the given window. A non-positive
// interval defaults to 1s.
func NewThrottle(interval time.Duration) *Throttle {
	if interval <= 0 {
		interval = time.Second
	}
	return &Throttle{interval: interval}
}

// Allow reports whether the current occurrence may be logged, and how many
// occurrences were suppressed since the previously allowed one.
func (t *Throttle) Allow() (allowed bool, suppressed int) {
	if t == nil {
		return true, 0
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	now := time.Now()
	if !t.last.IsZero() && now.Sub(t.last) < t.interval {
		t.suppressed++
		return false, 0
	}
	t.last = now
	n := t.suppressed
	t.suppressed = 0
	return true, n
}
