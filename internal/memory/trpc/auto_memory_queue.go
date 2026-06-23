package trpcmem

// H-01: MemoryJobPriority, MemoryDeadLetterReason, MemoryDeadLetterSink are now
// canonical in internal/biz (memory_queue_contract.go). We re-export them here as
// type aliases so existing call sites in this package compile unchanged.

import (
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/conf"
	"aranea-agents/pkg/appctx"
	"aranea-agents/pkg/loggateway"
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
// The contract requires consumers to call AckDone after processing each job so that
// per-tenant in-flight quotas are released. Failure to call AckDone causes tenant
// slots to leak, eventually blocking all Normal-priority jobs for that tenant.
type AutoMemoryQueue interface {
	// Enqueue adds a job to the queue. Normal-priority jobs are subject to
	// per-tenant quota; exceeding the quota dead-letters the job.
	Enqueue(r AutoMemoryJobRequest)
	// Chan returns the merged output channel consumed by workers.
	Chan() <-chan AutoMemoryJobRequest
	// AckDone must be called by the consumer after it finishes processing a
	// job (whether successfully or not). It decrements the per-tenant in-flight
	// counter that was reserved at Enqueue time. Calls for non-Normal-priority
	// jobs are no-ops.
	AckDone(r AutoMemoryJobRequest)
}

// MemoryJobQueue is a three-priority memory job queue (MEM-OPT-03).
// It replaces the previous single-channel implementation while keeping the
// AutoMemoryQueue interface unchanged (Chan() returns a merged output channel).
type MemoryJobQueue struct {
	high   chan AutoMemoryJobRequest
	normal chan AutoMemoryJobRequest
	low    chan AutoMemoryJobRequest
	out    chan AutoMemoryJobRequest

	recent   sync.Map
	debounce time.Duration

	dropped   atomic.Int64
	debounced atomic.Int64

	mu             sync.Mutex
	tenantInFlight map[string]int64

	memConf conf.RuntimeMemoryQueueConfig

	deadLetter MemoryDeadLetterSink

	done chan struct{}
	wg   sync.WaitGroup

	lg loggateway.Logger
}

var _ AutoMemoryQueue = (*MemoryJobQueue)(nil)

// NewMemoryJobQueue creates a priority-aware MemoryJobQueue.
// size is ignored (kept for API compatibility); use the named-priority capacities instead.
// // WIRE: needs *conf.Runtime
func NewMemoryJobQueue(runtimeConf *conf.Runtime, size int, debounce time.Duration, lg loggateway.Logger) *MemoryJobQueue {
	memConf := runtimeConf.MemoryQueueConfig()
	if debounce <= 0 {
		debounce = memConf.Debounce
	}
	q := &MemoryJobQueue{
		high:           make(chan AutoMemoryJobRequest, memConf.HighCap),
		normal:         make(chan AutoMemoryJobRequest, memConf.NormalCap),
		low:            make(chan AutoMemoryJobRequest, memConf.LowCap),
		out:            make(chan AutoMemoryJobRequest, memConf.NormalCap),
		debounce:       debounce,
		tenantInFlight: make(map[string]int64),
		memConf:        memConf,
		done:           make(chan struct{}),
		lg:             lg,
	}
	q.wg.Add(2)
	safego.Go(appctx.Ctx(), "memory.job_queue.drain", q.drain)
	safego.Go(appctx.Ctx(), "memory.job_queue.cleanup_recent", q.cleanupRecent)
	return q
}

// Close shuts down background goroutines and waits for them to exit.
// Call during graceful shutdown.
func (q *MemoryJobQueue) Close() {
	if q == nil {
		return
	}
	close(q.done)
	q.wg.Wait()
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
		q.lg.Warn("auto-memory job dropped → dead-letter",
			loggateway.StepID("memory.queue.drop"),
			loggateway.Str("reason", string(reason)),
			loggateway.Int("total_dropped", int(n)),
			loggateway.Str("session_id", r.SessionID),
			loggateway.Any("priority", r.Priority))
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
		if inFlight >= int64(q.memConf.MaxTenantNormalSlots) {
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
// Respects q.done for graceful shutdown (H-03). Every send to q.out uses
// a select on q.done so that a full output channel cannot block shutdown.
func (q *MemoryJobQueue) drain() {
	defer q.wg.Done()
	defer close(q.out)
	const lowBatchMax = 4
	// sendOut writes to q.out while respecting q.done for graceful shutdown.
	// Returns false if shutdown was signalled.
	sendOut := func(r AutoMemoryJobRequest) bool {
		select {
		case q.out <- r:
			return true
		case <-q.done:
			return false
		}
	}

	for {
		// Always drain high first (non-blocking).
		select {
		case r := <-q.high:
			if !sendOut(r) {
				return
			}
			continue
		default:
		}
		// Then normal (non-blocking).
		select {
		case r := <-q.normal:
			if !sendOut(r) {
				return
			}
			continue
		default:
		}
		// Drain up to lowBatchMax low items, then fall through to blocking select.
		drained := 0
		for drained < lowBatchMax {
			select {
			case r := <-q.low:
				if !sendOut(r) {
					return
				}
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
			if !sendOut(r) {
				return
			}
		case r := <-q.normal:
			if !sendOut(r) {
				return
			}
		case r := <-q.low:
			if !sendOut(r) {
				return
			}
		}
	}
}

// cleanupRecent periodically removes stale debounce entries to prevent memory
// accumulation from long-lived sessions (H-05).
func (q *MemoryJobQueue) cleanupRecent() {
	defer q.wg.Done()
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
	return s.HighLen, s.NormalLen, s.LowLen, int(q.memConf.HighCap), int(q.memConf.NormalCap), int(q.memConf.LowCap), s.Dropped, s.Debounced
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
// Feedback jobs are high priority and do not carry appName/userID at enqueue time
// (the caller only knows sessionID + messageID). The extractFeedback worker resolves
// these from the session at processing time. TenantID is left empty so that
// high-priority jobs bypass the per-tenant normal-slot quota.
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
