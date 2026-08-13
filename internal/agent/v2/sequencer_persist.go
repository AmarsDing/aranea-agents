package v2

// sequencer_persist.go — 持久化子管理器（TECH-DEBT(COG) 拆分，P1-1）。
//
// 从 Sequencer 拆出的持久化职责：persistChan 消费、指数退避重试、
// 死信环形缓冲 + 可选持久化死信存储 + 后台 replay worker。
// Sequencer 仅保留发布路径与门面 API；本文件的方法不对外暴露。

import (
	"context"
	"sync"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
)

// deadLetterErrLogInterval is the throttle window for the dead-letter Error
// log (see sequencerThrottles.deadLetterErr).
const deadLetterErrLogInterval = 10 * time.Second

// stallWarnLogInterval is the throttle window for the per-event stall Warns
// on the persist failure chain (R2 persist-channel-full, R3 dead-letter
// save, R4 outbox insert).
const stallWarnLogInterval = 10 * time.Second

// sequencerThrottles groups the five per-failure-chain log throttles.
// Behaviour (dead-lettering, outbox skip) is never throttled — only logs.
type sequencerThrottles struct {
	// deadLetterErr throttles the "persist exhausted retries" Error.
	deadLetterErr *loggateway.Throttle
	// enqueueTimeoutErr / persistFullWarn / deadLetterSaveWarn /
	// outboxInsertWarn throttle the remaining per-event logs on the same
	// pipeline-stall failure chain (R1-R4).
	enqueueTimeoutErr  *loggateway.Throttle
	persistFullWarn    *loggateway.Throttle
	deadLetterSaveWarn *loggateway.Throttle
	outboxInsertWarn   *loggateway.Throttle
}

func newSequencerThrottles() *sequencerThrottles {
	return &sequencerThrottles{
		deadLetterErr:      loggateway.NewThrottle(deadLetterErrLogInterval),
		enqueueTimeoutErr:  loggateway.NewThrottle(deadLetterErrLogInterval),
		persistFullWarn:    loggateway.NewThrottle(stallWarnLogInterval),
		deadLetterSaveWarn: loggateway.NewThrottle(stallWarnLogInterval),
		outboxInsertWarn:   loggateway.NewThrottle(stallWarnLogInterval),
	}
}

// stallFields appends the suppressed-failure count when a throttle window
// reset since the previous emission, so throttled-away failures stay visible
// as a count instead of vanishing silently.
func stallFields(suppressed int, fields ...loggateway.Field) []loggateway.Field {
	if suppressed > 0 {
		fields = append(fields, loggateway.Int("suppressed", suppressed))
	}
	return fields
}

// persistWorker owns the async persist pipeline: channel consumption, retry
// with exponential backoff, dead-letter ring + optional durable store, and
// the background replay worker.
type persistWorker struct {
	repoSet RepoSet
	lg      loggateway.Logger

	ch         chan persistItem
	wg         sync.WaitGroup
	maxRetries int
	backoff    time.Duration

	ring *deadLetterRing
	// store is the optional durable dead-letter store (P1-R2b). When set,
	// dead-lettered events are also written to the event_dead_letter table
	// and replayed (startup + periodic) until success or attempt cap.
	store biz.EventDeadLetterRepo

	replayDone    chan struct{}
	replayWG      sync.WaitGroup
	replayStarted bool // 仅当后台 replay worker 启动时为 true（close 据此收尾）

	throttles *sequencerThrottles
}

func newPersistWorker(rs RepoSet, lg loggateway.Logger, throttles *sequencerThrottles, cfg config) *persistWorker {
	return &persistWorker{
		repoSet:    rs,
		lg:         lg,
		ch:         make(chan persistItem, cfg.persistBuffer),
		maxRetries: cfg.persistMaxRetries,
		backoff:    cfg.persistBackoff,
		ring:       newDeadLetterRing(cfg.deadLetterCapacity),
		store:      cfg.deadLetterStore,
		replayDone: make(chan struct{}),
		throttles:  throttles,
	}
}

// start launches the persist loop and (unless disabled for tests) the
// dead-letter replay worker. 红线 #13：管道 worker 必须走 safego。
func (p *persistWorker) start(disableReplayLoop bool) {
	p.wg.Add(1)
	safego.GoBackground("sequencer-v2-persist", p.persistLoop)
	if p.store != nil && !disableReplayLoop {
		p.replayStarted = true
		p.replayWG.Add(1)
		safego.GoBackground("sequencer-v2-dl-replay", p.deadLetterReplayLoop)
	}
}

// close drains the persist loop then stops the replay worker.
// Caller must guarantee no producer sends to ch after this point
// (Sequencer.Close closes publishQueue and waits publishWG first).
func (p *persistWorker) close() {
	close(p.ch)
	p.wg.Wait()
	if p.replayStarted {
		close(p.replayDone)
		p.replayWG.Wait()
	}
}

// persistLoop consumes ch with retry + dead-letter.
func (p *persistWorker) persistLoop() {
	defer p.wg.Done()
	for item := range p.ch {
		if item.flushCh != nil {
			close(item.flushCh)
			continue
		}
		p.persistWithRetry(item.event)
	}
}

func (p *persistWorker) persistWithRetry(e biz.Event) {
	var lastErr error
	for attempt := 0; attempt < p.maxRetries; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_, err := persistAction(ctx, p.repoSet, e)
		cancel()
		if err == nil {
			return
		}
		lastErr = err
		// Exponential backoff: 1x, 2x, 4x, 8x, 16x.
		// Y7: no sleep after the final attempt — the event is about to be
		// dead-lettered, so the extra backoff only delays the failure signal.
		if attempt < p.maxRetries-1 {
			time.Sleep(p.backoff * time.Duration(1<<attempt))
		}
	}
	p.pushDeadLetterThrottledLog(e, lastErr)
	p.pushDeadLetter(e)
}

// pushDeadLetterThrottledLog emits the "persist exhausted retries" Error at
// most once per deadLetterErrLogInterval, attaching the suppressed count when
// the window resets. Dead-lettering itself is never throttled.
func (p *persistWorker) pushDeadLetterThrottledLog(e biz.Event, lastErr error) {
	ok, suppressed := p.throttles.deadLetterErr.Allow()
	if !ok {
		return
	}
	fields := []loggateway.Field{loggateway.Str("kind", string(e.EventKind())), loggateway.Err(lastErr)}
	if suppressed > 0 {
		fields = append(fields, loggateway.Int("suppressed", suppressed))
	}
	p.lg.Error("persist exhausted retries, sending to dead-letter", fields...)
}

// deadLetterRing is a FIFO ring buffer with entity-ID-based dedup.
type deadLetterRing struct {
	mu   sync.Mutex
	buf  []biz.Event
	cap  int
	seen map[string]struct{} // dedup by entity ID (extracted from event)
}

func newDeadLetterRing(capacity int) *deadLetterRing {
	return &deadLetterRing{
		buf:  make([]biz.Event, 0, capacity),
		cap:  capacity,
		seen: make(map[string]struct{}),
	}
}

func (r *deadLetterRing) Push(e biz.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	id := deadLetterID(e)
	if _, ok := r.seen[id]; ok {
		// A5: same entity already dead-lettered — replace the stale entry with
		// the newest event (aligns with the durable store's upsert semantics;
		// replay/inspection must see the latest entity state, not an old one).
		for i, existing := range r.buf {
			if deadLetterID(existing) == id {
				r.buf = append(r.buf[:i], r.buf[i+1:]...)
				break
			}
		}
	}
	if len(r.buf) >= r.cap {
		// Evict oldest
		old := r.buf[0]
		delete(r.seen, deadLetterID(old))
		r.buf = r.buf[1:]
	}
	r.buf = append(r.buf, e)
	r.seen[id] = struct{}{}
}

func (r *deadLetterRing) Len() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.buf)
}

// deadLetterID extracts the entity ID for deduplication.
// Uses the EntityID() method added to the Event interface (Deviation 7).
func deadLetterID(e biz.Event) string {
	return e.EntityID()
}
