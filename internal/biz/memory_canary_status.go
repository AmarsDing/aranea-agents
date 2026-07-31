package biz

import (
	"sync/atomic"
	"time"
)

// MemoryCanarySnapshot is a point-in-time view of the memory canary health.
type MemoryCanarySnapshot struct {
	RunsTotal           int64
	FailedTotal         int64
	ConsecutiveFailures int64
	LastRunUnix         int64
	LastOKUnix          int64
	LastFailStage       string
	LastFailReason      string
}

// MemoryCanaryStatus tracks the write → recall → archive canary loop health.
// It is the in-memory metric source for the monitor alert metric
// (memory.canary_consecutive_failures) and the future panel health card.
// All methods are nil-safe and goroutine-safe.
type MemoryCanaryStatus struct {
	runsTotal           atomic.Int64
	failedTotal         atomic.Int64
	consecutiveFailures atomic.Int64
	lastRunUnix         atomic.Int64
	lastOKUnix          atomic.Int64
	lastFailStage       atomic.Value // string
	lastFailReason      atomic.Value // string
}

// NewMemoryCanaryStatus creates a new status tracker for DI injection.
func NewMemoryCanaryStatus() *MemoryCanaryStatus {
	s := &MemoryCanaryStatus{}
	s.lastFailStage.Store("")
	s.lastFailReason.Store("")
	return s
}

// RecordOK marks one full canary loop as passed.
func (s *MemoryCanaryStatus) RecordOK() {
	if s == nil {
		return
	}
	now := time.Now().UTC().Unix()
	s.runsTotal.Add(1)
	s.consecutiveFailures.Store(0)
	s.lastRunUnix.Store(now)
	s.lastOKUnix.Store(now)
}

// RecordFail marks one canary loop as failed at the given stage
// ("write" | "recall" | "archive") with a short reason for diagnosis.
func (s *MemoryCanaryStatus) RecordFail(stage, reason string) {
	if s == nil {
		return
	}
	s.runsTotal.Add(1)
	s.failedTotal.Add(1)
	s.consecutiveFailures.Add(1)
	s.lastRunUnix.Store(time.Now().UTC().Unix())
	s.lastFailStage.Store(stage)
	s.lastFailReason.Store(reason)
}

// ConsecutiveFailures returns the current consecutive failure streak.
// The monitor alert metric fires when this is >= 1.
func (s *MemoryCanaryStatus) ConsecutiveFailures() int64 {
	if s == nil {
		return 0
	}
	return s.consecutiveFailures.Load()
}

// Snapshot returns a consistent view of all counters.
func (s *MemoryCanaryStatus) Snapshot() MemoryCanarySnapshot {
	if s == nil {
		return MemoryCanarySnapshot{}
	}
	stage, _ := s.lastFailStage.Load().(string)
	reason, _ := s.lastFailReason.Load().(string)
	return MemoryCanarySnapshot{
		RunsTotal:           s.runsTotal.Load(),
		FailedTotal:         s.failedTotal.Load(),
		ConsecutiveFailures: s.consecutiveFailures.Load(),
		LastRunUnix:         s.lastRunUnix.Load(),
		LastOKUnix:          s.lastOKUnix.Load(),
		LastFailStage:       stage,
		LastFailReason:      reason,
	}
}
