package service

import (
	"testing"
	"time"
)

// Review #8: clarificationPendingCache.Sweep must drop only entries idle
// longer than clarificationPendingMaxIdle, keeping fresh clarify cards hot.
func TestClarificationPendingCache_SweepExpiresStaleEntries(t *testing.T) {
	c := &clarificationPendingCache{}
	c.Store("fresh", pendingClarification{StepID: "s1", CreatedAt: time.Now()})
	stale := pendingClarification{StepID: "s2", CreatedAt: time.Now().Add(-2 * clarificationPendingMaxIdle)}
	c.Store("stale", stale)

	c.Sweep()

	if _, ok := c.Load("fresh"); !ok {
		t.Fatal("fresh entry must survive the sweep")
	}
	if _, ok := c.Load("stale"); ok {
		t.Fatal("stale entry must be swept")
	}
}

// A nil cache must be Sweep-safe (mirrors the other nil-guarded methods).
func TestClarificationPendingCache_SweepNilSafe(t *testing.T) {
	var c *clarificationPendingCache
	c.Sweep()
}
