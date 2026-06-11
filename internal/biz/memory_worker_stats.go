package biz

import (
	"sync/atomic"
)

// MemoryWorkerStats tracks auto-memory pipeline counters for admin status RPC.
type MemoryWorkerStats struct {
	jobsDone       atomic.Int64
	jobsDead       atomic.Int64
	llmFallback    atomic.Int64
	extractTotalMs atomic.Int64
	extractCount   atomic.Int64
	backfillTotal  atomic.Int64
}

// NewMemoryWorkerStats creates a new MemoryWorkerStats instance for DI injection.
func NewMemoryWorkerStats() *MemoryWorkerStats {
	return &MemoryWorkerStats{}
}

// globalMemoryWorkerStats is the process-level default stats instance.
// Deprecated: use NewMemoryWorkerStats() + Wire injection instead of MemoryWorkerStatsGlobal().
var globalMemoryWorkerStats MemoryWorkerStats

// MemoryWorkerStatsGlobal returns the process-level stats singleton.
// Deprecated: inject *MemoryWorkerStats via Wire instead of using this global accessor.
func MemoryWorkerStatsGlobal() *MemoryWorkerStats { return &globalMemoryWorkerStats }

func (s *MemoryWorkerStats) RecordJobDone(durationMs int64) {
	if s == nil {
		return
	}
	s.jobsDone.Add(1)
	if durationMs > 0 {
		s.extractTotalMs.Add(durationMs)
		s.extractCount.Add(1)
	}
}

func (s *MemoryWorkerStats) RecordJobDead() {
	if s != nil {
		s.jobsDead.Add(1)
	}
}

func (s *MemoryWorkerStats) RecordLLMFallback() {
	if s != nil {
		s.llmFallback.Add(1)
	}
}

func (s *MemoryWorkerStats) RecordEpisodeBackfill(n int64) {
	if s != nil && n > 0 {
		s.backfillTotal.Add(n)
	}
}

func (s *MemoryWorkerStats) Snapshot() (done, dead, fallback, backfill int64, avgExtractSec float64) {
	if s == nil {
		return 0, 0, 0, 0, 0
	}
	done = s.jobsDone.Load()
	dead = s.jobsDead.Load()
	fallback = s.llmFallback.Load()
	backfill = s.backfillTotal.Load()
	cnt := s.extractCount.Load()
	if cnt > 0 {
		avgExtractSec = float64(s.extractTotalMs.Load()) / float64(cnt) / 1000
	}
	return
}
