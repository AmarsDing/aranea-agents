package v2

import (
	"sync"
	"sync/atomic"
)

// defaultSeqAssigner is a v2-local SeqAssigner implementation that maintains a
// per-spirit-session atomic counter. It is functionally equivalent to
// internal/agent.SeqAssigner but lives in the v2 package to break the circular
// import that would otherwise arise when internal/agent imports v2 (for the
// dual-path stream consumer) while v2/sequencer.go previously imported
// internal/agent for *agent.SeqAssigner.
//
// The v1 agent.SeqAssigner remains the canonical implementation for v1 paths;
// v2 has its own independent counter space so the two paths do not interfere.
//
// sync.Map is used (1 per struct) consistent with AS-COG-01; this struct is the
// single-responsibility Seq allocator, not a business struct, so the
// sync.Map extraction rule does not apply.
type defaultSeqAssigner struct {
	counters sync.Map // sessionID → *atomic.Int64
}

// NewDefaultSeqAssigner constructs a v2-local defaultSeqAssigner.
func NewDefaultSeqAssigner() SeqAssigner {
	return &defaultSeqAssigner{}
}

// NextSeq returns the next monotonic Seq for the given spirit session (from 1).
// An empty spiritSessionID falls back to a "_default_" bucket to avoid
// polluting a single global counter.
func (s *defaultSeqAssigner) NextSeq(spiritSessionID string) int64 {
	if spiritSessionID == "" {
		spiritSessionID = "_default_"
	}
	v, _ := s.counters.LoadOrStore(spiritSessionID, &atomic.Int64{})
	return v.(*atomic.Int64).Add(1)
}

// RestoreAtLeast ensures the next NextSeq for spiritSessionID is strictly
// greater than minSeq. No-op when minSeq <= 0 or the counter is already ahead.
// B-06: call with MAX(seq) from persisted v2 entities after process restart.
func (s *defaultSeqAssigner) RestoreAtLeast(spiritSessionID string, minSeq int64) {
	if minSeq <= 0 {
		return
	}
	if spiritSessionID == "" {
		spiritSessionID = "_default_"
	}
	v, _ := s.counters.LoadOrStore(spiritSessionID, &atomic.Int64{})
	counter := v.(*atomic.Int64)
	for {
		cur := counter.Load()
		if cur >= minSeq {
			return
		}
		if counter.CompareAndSwap(cur, minSeq) {
			return
		}
	}
}
