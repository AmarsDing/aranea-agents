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
	// R3（2026-08-22）：trailing-edge 合并后不再为 debounced 写死信
	// （被合并的请求已并入存活请求，不是丢失）。常量保留用于读取存量 DB 行。
	DeadLetterReasonDebounced = biz.MemoryDeadLetterReasonDebounced
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

	// R3（2026-08-22）：trailing-edge debounce。每个 session 至多一条 pending
	// 条目，新请求替换旧请求（latest wins）并重置定时器；窗口静默期满后
	// 才入队。条目只在定时器挂起期间存在， firing/Close 时删除，因此不需要
	// 独立的 cleanup goroutine（取代旧 leading-edge 的 recent sync.Map +
	// cleanupRecent）。
	debounceMu      sync.Mutex
	debounceEntries map[string]*debounceEntry
	closed          bool
	debounce        time.Duration

	dropped   atomic.Int64
	debounced atomic.Int64 // 被更新请求合并取代的计数（观测用，不写死信）

	mu             sync.Mutex
	tenantInFlight map[string]int64

	memConf conf.RuntimeMemoryQueueConfig

	deadLetter MemoryDeadLetterSink

	done chan struct{}
	wg   sync.WaitGroup

	lg loggateway.Logger
}

// debounceEntry 是某 session 的挂起请求：req 始终是该 session 最新一条
// Enqueue 请求，timer 每次被取代时 Reset（trailing-edge）。
type debounceEntry struct {
	req   AutoMemoryJobRequest
	timer *time.Timer
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
		high:            make(chan AutoMemoryJobRequest, memConf.HighCap),
		normal:          make(chan AutoMemoryJobRequest, memConf.NormalCap),
		low:             make(chan AutoMemoryJobRequest, memConf.LowCap),
		out:             make(chan AutoMemoryJobRequest, memConf.NormalCap),
		debounceEntries: make(map[string]*debounceEntry),
		debounce:        debounce,
		tenantInFlight:  make(map[string]int64),
		memConf:         memConf,
		done:            make(chan struct{}),
		lg:              lg,
	}
	q.wg.Add(1)
	safego.Go(appctx.Ctx(), "memory.job_queue.drain", q.drain)
	return q
}

// Close flushes pending debounce entries (surviving requests are enqueued
// best-effort into the buffered normal lane; overflow goes to dead-letter),
// then shuts down background goroutines and waits for them to exit.
// Flush happens BEFORE signalling done so drain still has a chance to forward
// the surviving jobs during graceful shutdown. Idempotent. Call during
// graceful shutdown.
func (q *MemoryJobQueue) Close() {
	if q == nil {
		return
	}
	q.debounceMu.Lock()
	if q.closed {
		q.debounceMu.Unlock()
		return
	}
	q.closed = true
	pending := make([]AutoMemoryJobRequest, 0, len(q.debounceEntries))
	for sid, e := range q.debounceEntries {
		e.timer.Stop()
		pending = append(pending, e.req)
		delete(q.debounceEntries, sid)
	}
	q.debounceMu.Unlock()
	for _, r := range pending {
		q.enqueueNormalNow(r)
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
	// R3：normal 优先级且带 session 的请求走 trailing-edge 合并——
	// 窗口内只留最新一条，静默期满才真正入队。无 session 的请求无法
	// 按会话合并，保持即时入队。
	if r.Priority == MemoryJobPriorityNormal {
		if sid := strings.TrimSpace(r.SessionID); sid != "" {
			q.coalesceNormal(sid, r)
			return
		}
		q.enqueueNormalNow(r)
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

// coalesceNormal 实现 trailing-edge debounce：session 首条请求武装定时器；
// 窗口内的后续请求替换 pending 请求（latest wins）并重置定时器。每段静默期
// 恰好入队一条（突发平息之后）。被合并的请求不写死信（R3）：它们已并入
// 存活请求，不是丢失——旧实现写 debounced 死信会被 replayer 重新入队，
// 等于去抖失效且徒增重复抽取。
func (q *MemoryJobQueue) coalesceNormal(sid string, r AutoMemoryJobRequest) {
	q.debounceMu.Lock()
	if q.closed {
		// Close 已 flush 全部 pending 条目；晚到的请求直接走入队路径，
		// 保留配额/背压语义，而不是武装一个无人观察的定时器。
		q.debounceMu.Unlock()
		q.enqueueNormalNow(r)
		return
	}
	if e, ok := q.debounceEntries[sid]; ok {
		e.req = r
		e.timer.Reset(q.debounce)
		q.debounced.Add(1)
		q.debounceMu.Unlock()
		return
	}
	e := &debounceEntry{req: r}
	e.timer = time.AfterFunc(q.debounce, func() { q.fireDebounced(sid, e) })
	q.debounceEntries[sid] = e
	q.debounceMu.Unlock()
}

// fireDebounced 在静默窗口期满时入队存活请求。entry 指针守卫使过期定时器
// 触发（条目已被 Close flush 或 firing 与 Reset 竞争）成为 no-op，
// 保证一条合并序列至多入队一次。
func (q *MemoryJobQueue) fireDebounced(sid string, e *debounceEntry) {
	q.debounceMu.Lock()
	cur, ok := q.debounceEntries[sid]
	if !ok || cur != e {
		q.debounceMu.Unlock()
		return
	}
	delete(q.debounceEntries, sid)
	req := e.req
	q.debounceMu.Unlock()
	q.enqueueNormalNow(req)
}

// enqueueNormalNow 是 normal 优先级的实际入队路径：租户配额校验（C-02：
// 成功发送才保留占位）+ 非阻塞发送，溢出写死信。
func (q *MemoryJobQueue) enqueueNormalNow(r AutoMemoryJobRequest) {
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
	// R3：先非阻塞投递——done 已关闭但 out 仍有缓冲时也必须把已取出的
	// 任务送达，否则「从 lane 取出却丢在 sendOut」会在关停期丢任务
	// （Close flush 路径依赖此语义）。仅当 out 真满（背压）才退回
	// done 感知的阻塞等待。
	sendOut := func(r AutoMemoryJobRequest) bool {
		select {
		case q.out <- r:
			return true
		default:
		}
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
