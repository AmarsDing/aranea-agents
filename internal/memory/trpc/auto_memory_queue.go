package trpcmem

// H-01: MemoryJobPriority, MemoryDeadLetterReason, MemoryDeadLetterSink are now
// canonical in internal/biz (memory_queue_contract.go). We re-export them here as
// type aliases so existing call sites in this package compile unchanged.

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/pkg/safego"
)

// Re-export biz contracts as local aliases (no copy of logic).
type MemoryJobPriority = biz.MemoryJobPriority
type MemoryDeadLetterReason = biz.MemoryDeadLetterReason

const (
	MemoryJobPriorityHigh   = biz.MemoryJobPriorityHigh
	MemoryJobPriorityNormal = biz.MemoryJobPriorityNormal
	MemoryJobPriorityLow    = biz.MemoryJobPriorityLow

	DeadLetterReasonQueueFull     = biz.MemoryDeadLetterReasonQueueFull
	DeadLetterReasonQuotaExceeded = biz.MemoryDeadLetterReasonQuotaExceeded
)

// MemoryDeadLetterSink is the biz-layer contract re-exported for adapter use.
type MemoryDeadLetterSink = biz.MemoryDeadLetterSink

type AutoMemoryJobRequest struct {
	AppName    string
	SessionID  string
	UserID     string
	EnqueuedAt time.Time
	// Feedback-triggered preference extraction (optional).
	FeedbackMessageID string
	FeedbackRating    string
	FeedbackComment   string
	// MEM-OPT-03: priority and tenant/workspace routing.
	Priority MemoryJobPriority
	TenantID string // defaults to AppName if empty
}

// AutoMemoryQueue abstracts the job queue consumed by AutoMemoryWorker and sqlite memory service.
type AutoMemoryQueue interface {
	Enqueue(r AutoMemoryJobRequest)
	Chan() <-chan AutoMemoryJobRequest
}

// MemoryJobQueue default capacities (MEM-OPT-03 priority lanes).
const (
	memQueueHighCap   = 64
	memQueueNormalCap = 256
	memQueueLowCap    = 128

	// maxTenantNormalSlots is the max share of the normal queue any single tenant may hold.
	// Jobs beyond this are written to dead-letter (quota_exceeded).
	maxTenantNormalSlots = 128
)

// MemoryJobQueue is a three-priority memory job queue (MEM-OPT-03).
// It replaces the previous single-channel implementation while keeping the
// AutoMemoryQueue interface unchanged (Chan() returns a merged output channel).
type MemoryJobQueue struct {
	high   chan AutoMemoryJobRequest // cap 64 — feedback / preference
	normal chan AutoMemoryJobRequest // cap 256 — runner turn completion
	low    chan AutoMemoryJobRequest // cap 128 — backfill / reconcile
	out    chan AutoMemoryJobRequest // merged output (cap=normalCap to avoid double-buffering)

	recent   sync.Map
	debounce time.Duration

	dropped   atomic.Int64
	debounced atomic.Int64

	// tenant in-flight tracking (C-02): incremented on successful enqueue,
	// decremented via AckDone() called by the Worker after processing each job.
	mu             sync.Mutex
	tenantInFlight map[string]int64

	deadLetter MemoryDeadLetterSink // optional; nil disables dead-letter persistence

	// H-03: shutdown signal for drain + cleanup goroutines.
	done chan struct{}
}

var _ AutoMemoryQueue = (*MemoryJobQueue)(nil)

// NewMemoryJobQueue creates a priority-aware MemoryJobQueue.
// size is ignored (kept for API compatibility); use the named-priority capacities instead.
func NewMemoryJobQueue(size int, debounce time.Duration) *MemoryJobQueue {
	if debounce <= 0 {
		debounce = 30 * time.Second
	}
	q := &MemoryJobQueue{
		high:           make(chan AutoMemoryJobRequest, memQueueHighCap),
		normal:         make(chan AutoMemoryJobRequest, memQueueNormalCap),
		low:            make(chan AutoMemoryJobRequest, memQueueLowCap),
		out:            make(chan AutoMemoryJobRequest, memQueueNormalCap), // M-06: was 448, now 256
		debounce:       debounce,
		tenantInFlight: make(map[string]int64),
		done:           make(chan struct{}),
	}
	safego.Go(context.Background(), "memory.job_queue.drain", q.drain)
	safego.Go(context.Background(), "memory.job_queue.cleanup_recent", q.cleanupRecent)
	return q
}

// Close shuts down background goroutines. Call during graceful shutdown.
func (q *MemoryJobQueue) Close() {
	if q == nil {
		return
	}
	close(q.done)
}

// SetDeadLetterSink wires a persistent dead-letter store (MEM-OPT-03).
func (q *MemoryJobQueue) SetDeadLetterSink(sink MemoryDeadLetterSink) {
	if q == nil {
		return
	}
	q.deadLetter = sink
}

func (q *MemoryJobQueue) tenantID(r AutoMemoryJobRequest) string {
	if t := strings.TrimSpace(r.TenantID); t != "" {
		return t
	}
	if a := strings.TrimSpace(r.AppName); a != "" {
		return a
	}
	return "default"
}

func (q *MemoryJobQueue) writeDeadLetter(r AutoMemoryJobRequest, reason MemoryDeadLetterReason) {
	n := q.dropped.Add(1)
	if n == 1 || n%10 == 0 {
		event.SysLogWarn("system.auto_memory.extract_fail", "auto-memory job dropped → dead-letter",
			event.P("reason", string(reason)),
			event.P("total_dropped", n),
			event.P("session_id", r.SessionID),
			event.P("priority", r.Priority),
		)
	}
	if q.deadLetter != nil {
		q.deadLetter.WriteMemoryDeadLetter(biz.MemoryDeadLetterRequest{
			SessionID:         r.SessionID,
			AppName:           r.AppName,
			UserID:            r.UserID,
			FeedbackMessageID: r.FeedbackMessageID,
			FeedbackRating:    r.FeedbackRating,
			FeedbackComment:   r.FeedbackComment,
			Priority:          r.Priority,
			TenantID:          r.TenantID,
		}, reason, "")
	}
}

func (q *MemoryJobQueue) Enqueue(r AutoMemoryJobRequest) {
	if q == nil {
		return
	}
	if r.EnqueuedAt.IsZero() {
		r.EnqueuedAt = time.Now()
	}
	// Debounce normal-priority runner-turn jobs per session.
	if r.Priority == MemoryJobPriorityNormal {
		if sid := strings.TrimSpace(r.SessionID); sid != "" {
			if t, ok := q.recent.Load(sid); ok {
				if time.Since(t.(time.Time)) < q.debounce {
					q.debounced.Add(1)
					return
				}
			}
			q.recent.Store(sid, time.Now())
		}
	}

	// Tenant quota check on normal queue (C-02: also increment on success below).
	if r.Priority == MemoryJobPriorityNormal {
		tid := q.tenantID(r)
		q.mu.Lock()
		inFlight := q.tenantInFlight[tid]
		if inFlight >= maxTenantNormalSlots {
			q.mu.Unlock()
			q.writeDeadLetter(r, DeadLetterReasonQuotaExceeded)
			return
		}
		// Reserve the slot before releasing the lock to prevent TOCTOU race.
		q.tenantInFlight[tid]++
		q.mu.Unlock()
		select {
		case q.normal <- r:
		default:
			// Queue full: undo the reservation and dead-letter.
			q.mu.Lock()
			if q.tenantInFlight[tid] > 0 {
				q.tenantInFlight[tid]--
			}
			q.mu.Unlock()
			q.writeDeadLetter(r, DeadLetterReasonQueueFull)
		}
		return
	}

	var ch chan AutoMemoryJobRequest
	switch r.Priority {
	case MemoryJobPriorityHigh:
		ch = q.high
	default: // MemoryJobPriorityLow
		ch = q.low
	}
	select {
	case ch <- r:
	default:
		q.writeDeadLetter(r, DeadLetterReasonQueueFull)
	}
}

// AckDone must be called by the Worker after it finishes processing a job that
// was dequeued from Chan(). It decrements the per-tenant in-flight counter (C-02).
func (q *MemoryJobQueue) AckDone(r AutoMemoryJobRequest) {
	if q == nil || r.Priority != MemoryJobPriorityNormal {
		return
	}
	tid := q.tenantID(r)
	q.mu.Lock()
	if q.tenantInFlight[tid] > 0 {
		q.tenantInFlight[tid]--
		if q.tenantInFlight[tid] == 0 {
			delete(q.tenantInFlight, tid) // keep map compact
		}
	}
	q.mu.Unlock()
}

// drain merges the three priority channels into q.out in priority order.
// Respects q.done for graceful shutdown (H-03).
func (q *MemoryJobQueue) drain() {
	const lowBatchMax = 4
	for {
		// Always drain high first (non-blocking).
		select {
		case r := <-q.high:
			q.out <- r
			continue
		default:
		}
		// Then normal (non-blocking).
		select {
		case r := <-q.normal:
			q.out <- r
			continue
		default:
		}
		// Drain up to lowBatchMax low items, then fall through to blocking select.
		drained := 0
		for drained < lowBatchMax {
			select {
			case r := <-q.low:
				q.out <- r
				drained++
			default:
				goto block
			}
		}
		continue
	block:
		// All queues empty — block until any of them has a new item or shutdown.
		select {
		case <-q.done:
			return
		case r := <-q.high:
			q.out <- r
		case r := <-q.normal:
			q.out <- r
		case r := <-q.low:
			q.out <- r
		}
	}
}

// cleanupRecent periodically removes stale debounce entries to prevent memory
// accumulation from long-lived sessions (H-05).
func (q *MemoryJobQueue) cleanupRecent() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-q.done:
			return
		case <-ticker.C:
			cutoff := time.Now().Add(-q.debounce * 2)
			q.recent.Range(func(key, value any) bool {
				if t, ok := value.(time.Time); ok && t.Before(cutoff) {
					q.recent.Delete(key)
				}
				return true
			})
		}
	}
}

// Chan returns the merged output channel consumed by AutoMemoryWorker.
func (q *MemoryJobQueue) Chan() <-chan AutoMemoryJobRequest {
	if q == nil {
		return nil
	}
	return q.out
}

// MemoryQueueStats captures per-priority queue depth and counters (MEM-OPT-03).
type MemoryQueueStats struct {
	HighLen   int
	NormalLen int
	LowLen    int
	OutLen    int
	Dropped   int64
	Debounced int64
}

func (q *MemoryJobQueue) Stats() (dropped, debounced int64) {
	if q == nil {
		return 0, 0
	}
	return q.dropped.Load(), q.debounced.Load()
}

// QueueStats returns richer observability data (MEM-OPT-03).
func (q *MemoryJobQueue) QueueStats() MemoryQueueStats {
	if q == nil {
		return MemoryQueueStats{}
	}
	return MemoryQueueStats{
		HighLen:   len(q.high),
		NormalLen: len(q.normal),
		LowLen:    len(q.low),
		OutLen:    len(q.out),
		Dropped:   q.dropped.Load(),
		Debounced: q.debounced.Load(),
	}
}

func (q *MemoryJobQueue) QueueLaneStats() (highLen, normalLen, lowLen int, highCap, normalCap, lowCap int, dropped, debounced int64) {
	s := q.QueueStats()
	return s.HighLen, s.NormalLen, s.LowLen, memQueueHighCap, memQueueNormalCap, memQueueLowCap, s.Dropped, s.Debounced
}

// NewAutoMemoryEnqueuer adapts a wired queue to biz.AutoMemoryEnqueuer (normal priority).
func NewAutoMemoryEnqueuer(q AutoMemoryQueue) func(appName, sessionID string, enqueuedAt time.Time) {
	return func(appName, sessionID string, enqueuedAt time.Time) {
		if q == nil {
			return
		}
		q.Enqueue(AutoMemoryJobRequest{
			AppName:    appName,
			SessionID:  sessionID,
			EnqueuedAt: enqueuedAt,
			Priority:   MemoryJobPriorityNormal,
			TenantID:   appName,
		})
	}
}

// NewFeedbackMemoryEnqueuer adapts a wired queue to biz.FeedbackMemoryEnqueuer (high priority).
func NewFeedbackMemoryEnqueuer(q AutoMemoryQueue) func(sessionID, messageID, rating, comment string, enqueuedAt time.Time) {
	return func(sessionID, messageID, rating, comment string, enqueuedAt time.Time) {
		if q == nil {
			return
		}
		// Feedback-triggered jobs are high priority (MEM-OPT-03).
		q.Enqueue(AutoMemoryJobRequest{
			SessionID:         sessionID,
			EnqueuedAt:        enqueuedAt,
			FeedbackMessageID: messageID,
			FeedbackRating:    rating,
			FeedbackComment:   comment,
			Priority:          MemoryJobPriorityHigh,
		})
	}
}
