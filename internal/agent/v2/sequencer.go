package v2

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"

	"github.com/google/uuid"
)

// Defaults — match v1 for parity.
const (
	defaultPublishBufferSize  = 256
	defaultPersistBufferSize  = 256
	defaultDeltaBatchInterval = 16 * time.Millisecond
	defaultPersistMaxRetries  = 5
	defaultPersistBackoff     = 100 * time.Millisecond
	defaultDeadLetterCapacity = 512
	// defaultPersistEnqueueTimeout 是持久化事件入队的保底等待上限：
	// 脱离 turn ctx 取消后仍等待该时长，队列持续打满才落死信。
	defaultPersistEnqueueTimeout = 5 * time.Second
	// deadLetterErrLogInterval 是死信 Error 日志的限流窗口（见
	// Sequencer.deadLetterErrThrottle）。
	deadLetterErrLogInterval = 10 * time.Second
)

// EventBus is the publish sink for v2 events (fan-out to WS subscribers).
// Structurally identical to biz.EventBus; kept as a local alias for clarity
// within the v2 package. Any biz.EventBus implementation satisfies it.
type EventBus interface {
	Publish(ctx context.Context, e biz.Event)
	Subscribe(opts biz.EventSubscribeOptions) (<-chan biz.Event, func())
}

// Sequencer is the single unified entry point for all v2 events.
// It replaces v1's dual-path (ActivityProjector→Sequencer + direct-publish).
//
// Invariants:
//  1. Single publish worker ensures FIFO across all event types.
//  2. step.streaming events are NOT persisted (only WS-published); same
//     StepID + DeltaField within the 16ms batch window are merged.
//  3. Persist worker: 5x exponential backoff retries + 512-cap dead-letter.
type Sequencer struct {
	repoSet     RepoSet
	outbox      biz.EventDeliveryOutboxRepo // B-06: durable critical-event outbox (optional)
	bus         EventBus
	lg          loggateway.Logger
	seqAssigner SeqAssigner // shared with Projector (v2-local defaultSeqAssigner; breaks agent→v2 cycle)

	publishQueue chan publishTask
	persistChan  chan persistItem
	deadLetter   *deadLetterRing

	// deadLetterErrThrottle limits the "persist exhausted retries" Error log.
	// When the DB is down, every persist-class event exhausts its retries and
	// would otherwise emit one Error per event for as long as the outage
	// lasts. Only the log line is throttled — dead-lettering itself is not.
	deadLetterErrThrottle *loggateway.Throttle

	// P1-R2b: optional durable dead-letter store. When set, dead-lettered
	// events are also written to the event_dead_letter table and replayed
	// (startup + periodic) until success or attempt cap.
	deadLetterStore biz.EventDeadLetterRepo
	replayDone      chan struct{}
	replayWG        sync.WaitGroup
	replayStarted   bool // 仅当后台 replay worker 启动时为 true（Close 据此收尾）

	publishWG sync.WaitGroup
	persistWG sync.WaitGroup

	deltaBatchInterval    time.Duration
	persistMaxRetries     int
	persistBackoff        time.Duration
	persistEnqueueTimeout time.Duration

	closed  atomic.Bool
	closeMu sync.Mutex
	// pubMu guards the check-closed-then-send critical section in Publish/Flush
	// against Close closing publishQueue (Y6: send-on-closed-channel panic).
	// Senders take RLock; Close takes Lock only around the channel close.
	pubMu sync.RWMutex
}

type publishTask struct {
	event      biz.Event
	persist    bool
	enqueuedAt time.Time
	flushCh    chan struct{} // non-nil for flush markers
}

type persistItem struct {
	event   biz.Event
	flushCh chan struct{} // non-nil for flush markers
}

type config struct {
	publishBuffer         int
	persistBuffer         int
	deltaBatchInterval    time.Duration
	persistMaxRetries     int
	persistBackoff        time.Duration
	persistEnqueueTimeout time.Duration
	deadLetterCapacity    int
	outbox                biz.EventDeliveryOutboxRepo
	deadLetterStore       biz.EventDeadLetterRepo
	// disableReplayLoop keeps the background dead-letter replay worker from
	// starting. Test-only: tests that drive replayDeadLettersOnce manually
	// must set this, otherwise the startup sweep races the manual call and
	// double-applies records.
	disableReplayLoop bool
}

// Option configures a Sequencer.
type Option func(*config)

func WithPublishBuffer(n int) Option { return func(c *config) { c.publishBuffer = n } }
func WithPersistBuffer(n int) Option { return func(c *config) { c.persistBuffer = n } }
func WithDeltaBatchInterval(d time.Duration) Option {
	return func(c *config) { c.deltaBatchInterval = d }
}
func WithPersistMaxRetries(n int) Option        { return func(c *config) { c.persistMaxRetries = n } }
func WithPersistBackoff(d time.Duration) Option { return func(c *config) { c.persistBackoff = d } }
func WithDeadLetterCapacity(n int) Option       { return func(c *config) { c.deadLetterCapacity = n } }

// WithPersistEnqueueTimeout overrides how long Publish waits for a persist-class
// event to enter publishQueue after the caller's ctx is detached (Y5). When the
// wait expires the event is dead-lettered (durable if a store is configured)
// instead of being silently dropped.
func WithPersistEnqueueTimeout(d time.Duration) Option {
	return func(c *config) { c.persistEnqueueTimeout = d }
}

// WithEventOutbox injects the durable critical-event outbox (B-06).
// When set, critical events are written to the outbox after entity persist
// (WBPF path) and before bus fan-out so WS reconnect can replay by last_event_id.
func WithEventOutbox(repo biz.EventDeliveryOutboxRepo) Option {
	return func(c *config) { c.outbox = repo }
}

// NewSequencer constructs a Sequencer and starts its publish + persist workers.
func NewSequencer(rs RepoSet, bus EventBus, lg loggateway.Logger, opts ...Option) *Sequencer {
	cfg := config{
		publishBuffer:         defaultPublishBufferSize,
		persistBuffer:         defaultPersistBufferSize,
		deltaBatchInterval:    defaultDeltaBatchInterval,
		persistMaxRetries:     defaultPersistMaxRetries,
		persistBackoff:        defaultPersistBackoff,
		persistEnqueueTimeout: defaultPersistEnqueueTimeout,
		deadLetterCapacity:    defaultDeadLetterCapacity,
	}
	for _, o := range opts {
		o(&cfg)
	}

	s := &Sequencer{
		repoSet:               rs,
		outbox:                cfg.outbox,
		bus:                   bus,
		lg:                    lg.With(loggateway.Domain("sequencer_v2")),
		seqAssigner:           NewDefaultSeqAssigner(),
		publishQueue:          make(chan publishTask, cfg.publishBuffer),
		persistChan:           make(chan persistItem, cfg.persistBuffer),
		deadLetter:            newDeadLetterRing(cfg.deadLetterCapacity),
		deadLetterErrThrottle: loggateway.NewThrottle(deadLetterErrLogInterval),
		deadLetterStore:       cfg.deadLetterStore,
		replayDone:            make(chan struct{}),
		deltaBatchInterval:    cfg.deltaBatchInterval,
		persistMaxRetries:     cfg.persistMaxRetries,
		persistBackoff:        cfg.persistBackoff,
		persistEnqueueTimeout: cfg.persistEnqueueTimeout,
	}

	s.publishWG.Add(1)
	// 红线 #13：管道 worker 必须走 safego。裸 goroutine 一旦 panic，事件管道
	// 会静默死亡（事件堆积至 buffer 满后所有发布者阻塞在 ctx.Done）；
	// safego 提供 recover + PanicHook 告警。
	safego.GoBackground("sequencer-v2-publish", s.publishLoop)
	s.persistWG.Add(1)
	safego.GoBackground("sequencer-v2-persist", s.persistLoop)
	if s.deadLetterStore != nil && !cfg.disableReplayLoop {
		s.replayStarted = true
		s.replayWG.Add(1)
		safego.GoBackground("sequencer-v2-dl-replay", s.deadLetterReplayLoop)
	}
	return s
}

// SeqAssigner exposes the shared SeqAssigner so Projector can pre-allocate Seq
// for turn-level events before publishing.
func (s *Sequencer) SeqAssigner() SeqAssigner { return s.seqAssigner }

// Publish enqueues an event for FIFO processing.
// Safe for concurrent use.
//
// 可靠性分级（Y2 修复）：
//   - 临时事件（streaming/notice/heartbeat/run_status，不落库）：turn ctx 取消时
//     允许丢弃——turn 已结束，残留 delta 无意义。
//   - 持久化事件（含 task/turn/step 终态）：不受 turn ctx 取消影响，脱离原 ctx
//     有限等待保底入队；仅当队列持续打满（管道停滞）超过 persistEnqueueTimeout
//     才落入死信（Y5：可重放，不再静默丢弃）。
func (s *Sequencer) Publish(ctx context.Context, e biz.Event) {
	// Y6: hold RLock across the closed-check + channel send so Close cannot
	// close publishQueue in between (send-on-closed-channel panic).
	s.pubMu.RLock()
	defer s.pubMu.RUnlock()
	if s.closed.Load() {
		s.lg.Warn("sequencer closed, event dropped", loggateway.Str("kind", string(e.EventKind())))
		return
	}
	if !s.shouldPersist(e) {
		select {
		case s.publishQueue <- publishTask{event: e, persist: false, enqueuedAt: time.Now()}:
		case <-ctx.Done():
			s.lg.Warn("publish ctx canceled before enqueue",
				loggateway.Str("kind", string(e.EventKind())), loggateway.Err(ctx.Err()))
		}
		return
	}
	enqueueCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), s.persistEnqueueTimeout)
	defer cancel()
	select {
	case s.publishQueue <- publishTask{event: e, persist: true, enqueuedAt: time.Now()}:
	case <-enqueueCtx.Done():
		// Y5: never silently drop a persist-class event — dead-letter it so the
		// ring/durable store can replay the entity upsert once the pipe recovers.
		s.lg.Error("persist event enqueue timeout, sending to dead-letter",
			loggateway.Str("kind", string(e.EventKind())), loggateway.Err(enqueueCtx.Err()))
		s.pushDeadLetter(e)
	}
}

// shouldPersist returns false for streaming chunks and ephemeral system events
// (only bus-published; no entity Upsert).
func (s *Sequencer) shouldPersist(e biz.Event) bool {
	switch e.(type) {
	case *biz.StepStreamingEvent, *biz.SystemNoticeEvent, *biz.HeartbeatEvent, *biz.RunStatusEvent, *biz.SkillCatalogEvent:
		return false
	default:
		return true
	}
}

// Flush blocks until all queued events are processed (publish + persist).
// Mainly for tests; production callers should rely on Close() for shutdown.
//
// Uses a flush marker enqueued into publishQueue: the marker is processed
// only after all prior tasks (including bus.Publish) have completed,
// establishing a happens-before relationship that eliminates data races
// between publishLoop and test assertions.
func (s *Sequencer) Flush(ctx context.Context) error {
	// Y6: hold RLock for the whole flush so Close cannot close publishQueue
	// (or persistChan, after publishLoop drains) while markers are in flight.
	s.pubMu.RLock()
	defer s.pubMu.RUnlock()
	if s.closed.Load() {
		return nil
	}
	flushCh := make(chan struct{})
	select {
	case s.publishQueue <- publishTask{flushCh: flushCh, enqueuedAt: time.Now()}:
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(5 * time.Second):
		return errors.New("sequencer flush: publish queue full")
	}
	select {
	case <-flushCh:
		// All publish tasks processed. Send persist flush marker.
		persistFlushCh := make(chan struct{})
		select {
		case s.persistChan <- persistItem{flushCh: persistFlushCh}:
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Second):
			return errors.New("sequencer flush: persist queue full")
		}
		select {
		case <-persistFlushCh:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Second):
			return errors.New("sequencer flush: persist timed out")
		}
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(5 * time.Second):
		return errors.New("sequencer flush timed out")
	}
}

// publishLoop is the single FIFO worker.
// It implements 16ms streaming batch merge for step.streaming events.
func (s *Sequencer) publishLoop() {
	defer s.publishWG.Done()
	var pendingStreaming *biz.StepStreamingEvent
	var pendingTimer *time.Timer
	var pendingDone chan struct{}

	for {
		select {
		case task, ok := <-s.publishQueue:
			if !ok {
				// Channel closed: drain any pending streaming event, then exit.
				if pendingStreaming != nil {
					if pendingTimer != nil {
						pendingTimer.Stop()
					}
					s.flushStreaming(pendingStreaming)
				}
				return
			}
			// Flush marker: all prior tasks have been fully processed.
			// Flush any pending streaming event, then signal the caller.
			if task.flushCh != nil {
				if pendingStreaming != nil {
					if pendingTimer != nil {
						pendingTimer.Stop()
					}
					s.flushStreaming(pendingStreaming)
					pendingStreaming = nil
					pendingDone = nil
					pendingTimer = nil
				}
				close(task.flushCh)
				continue
			}
			// If we have a pending streaming event and the new task is NOT a
			// mergeable streaming chunk, flush pending first.
			if pendingStreaming != nil {
				cur, isStreaming := task.event.(*biz.StepStreamingEvent)
				if !isStreaming || !canMergeStreaming(pendingStreaming, cur) {
					if pendingTimer != nil {
						pendingTimer.Stop()
					}
					s.flushStreaming(pendingStreaming)
					pendingStreaming = nil
					pendingDone = nil
					pendingTimer = nil
				} else {
					// Merge: accumulate delta content (Deviation 2: DeltaChunk not Delta)
					pendingStreaming.DeltaChunk += cur.DeltaChunk
					continue
				}
			}
			// Handle current event
			if ev, ok := task.event.(*biz.StepStreamingEvent); ok {
				// Start a new pending streaming with timer.
				// B1 fix: the timer callback must capture a LOCAL channel copy.
				// Capturing the shared pendingDone variable lets a late-firing
				// timer close the NEXT event's channel (premature flush) or nil
				// (panic → publishLoop dies → pipeline silently stalls).
				pendingStreaming = ev
				done := make(chan struct{})
				pendingDone = done
				pendingTimer = time.AfterFunc(s.deltaBatchInterval, func() {
					close(done)
				})
				continue
			}
			s.processTask(task)

		case <-pendingDone:
			if pendingStreaming != nil {
				s.flushStreaming(pendingStreaming)
				pendingStreaming = nil
				pendingDone = nil
				pendingTimer = nil
			}
		}
	}
}

// canMergeStreaming returns true iff two streaming events share StepID and DeltaField.
func canMergeStreaming(a, b *biz.StepStreamingEvent) bool {
	return a.StepID == b.StepID && a.DeltaField == b.DeltaField
}

// flushStreaming publishes the merged streaming event to bus only (no persist).
func (s *Sequencer) flushStreaming(ev *biz.StepStreamingEvent) {
	// P3 fix: assign a session-scoped monotonic DeltaSeq at the single flush
	// point (publishLoop is single-goroutine, so assignment is race-free) so
	// the frontend can dedup redelivered deltas by sequence instead of content
	// fingerprints. Assigned only once — a re-flushed event keeps its seq.
	if ev.DeltaSeq <= 0 && s.seqAssigner != nil {
		ev.DeltaSeq = s.seqAssigner.NextSeq(ev.SpiritSessionID())
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	s.bus.Publish(ctx, ev)
}

// processTask handles a non-mergeable event: persist + bus publish.
//
// P1-04 fix: terminal events (completed/failed/cancelled/interrupted/skipped)
// use write-before-publish (WBPF) — a single synchronous persist attempt is
// made BEFORE the bus publish. If the sync persist succeeds, the DB is
// guaranteed to have the terminal state before the frontend receives it. If
// the sync persist fails, the event falls back to the async persist path
// (with retries) and is still published — this is a best-effort WBPF that
// avoids leaving the frontend stuck in a non-terminal state when the DB is
// temporarily unavailable.
//
// Non-terminal events (created/updated/streaming) keep the original async
// persist + sync publish flow for low latency.
func (s *Sequencer) processTask(task publishTask) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if task.persist {
		if biz.IsCriticalDeliveryEvent(task.event) {
			// P1-04: WBPF for critical/terminal events — try synchronous persist first.
			//
			// Ordering fix (2026-07-20): the sync persist path bypasses
			// persistChan, so a terminal event (e.g. turn.completed,
			// Status=cancelled) could land in the repo BEFORE the still-queued
			// async persists of earlier non-terminal events (e.g. turn.started,
			// Status=running). The persistLoop would then overwrite the
			// terminal state with the stale non-terminal state. Drain
			// persistChan first (flush marker + wait) so every prior async
			// persist is applied before the terminal write.
			//
			// Deadlock-safe: publishLoop is the sole producer of persistChan
			// and persistLoop never sends back into publishQueue.
			drainCh := make(chan struct{})
			select {
			case s.persistChan <- persistItem{flushCh: drainCh}:
				select {
				case <-drainCh:
				case <-time.After(5 * time.Second):
					s.lg.Warn("terminal event persist-drain timed out, proceeding with sync persist",
						loggateway.Str("kind", string(task.event.EventKind())))
				}
			default:
				// persistChan full: persistLoop is backed up. Skip the drain;
				// the sync persist still runs (best-effort WBPF) and queued
				// items retain async retry semantics.
				s.lg.Warn("persist channel full, skipping pre-terminal drain",
					loggateway.Str("kind", string(task.event.EventKind())))
			}
			persistCtx, persistCancel := context.WithTimeout(context.Background(), 2*time.Second)
			_, err := persistAction(persistCtx, s.repoSet, task.event)
			persistCancel()
			if err == nil {
				// Sync persist succeeded — durable outbox then publish (B-06).
				s.publishCritical(ctx, task.event)
				return
			}
			// Sync persist failed — log and fall through to async retry + publish.
			s.lg.Warn("terminal event sync persist failed, falling back to async",
				loggateway.Str("kind", string(task.event.EventKind())),
				loggateway.Err(err))
		}
		// Async persist (non-terminal or terminal fallback).
		select {
		case s.persistChan <- persistItem{event: task.event}:
		default:
			// Persist channel full: log + drop to dead-letter.
			s.lg.Warn("persist channel full, event dropped to dead-letter",
				loggateway.Str("kind", string(task.event.EventKind())))
			s.pushDeadLetter(task.event)
		}
	}
	if biz.IsCriticalDeliveryEvent(task.event) {
		s.publishCritical(ctx, task.event)
		return
	}
	// Sync bus publish.
	s.bus.Publish(ctx, task.event)
}

// publishCritical writes the durable outbox row (best-effort) then fans out on the bus.
func (s *Sequencer) publishCritical(ctx context.Context, e biz.Event) {
	outboxID := s.insertCriticalOutbox(ctx, e)
	s.bus.Publish(ctx, e)
	if outboxID != "" && s.outbox != nil {
		if err := s.outbox.MarkPublished(ctx, outboxID, time.Now().UTC()); err != nil {
			s.lg.Warn("outbox mark published failed",
				loggateway.Str("kind", string(e.EventKind())),
				loggateway.Err(err))
		}
	}
}

// insertCriticalOutbox assigns seq when missing, inserts an outbox row, and returns its id.
// Failures are logged and ignored so publish is never fail-closed on outbox errors.
func (s *Sequencer) insertCriticalOutbox(ctx context.Context, e biz.Event) string {
	if s.outbox == nil || e == nil || !biz.IsCriticalDeliveryEvent(e) {
		return ""
	}
	sessionID := e.SpiritSessionID()
	if sessionID == "" {
		return ""
	}
	seq := biz.EventSeq(e)
	if seq <= 0 {
		if s.seqAssigner == nil {
			return ""
		}
		seq = s.seqAssigner.NextSeq(sessionID)
		biz.SetEventSeq(e, seq)
	}
	eventID := biz.DeliveryEventID(e, seq)
	payload, err := marshalV2EventEnvelope(e, eventID)
	if err != nil {
		s.lg.Warn("outbox marshal failed",
			loggateway.Str("kind", string(e.EventKind())),
			loggateway.Err(err))
		return ""
	}
	rowID := uuid.NewString()
	row := biz.EventDeliveryOutboxRow{
		ID:        rowID,
		SessionID: sessionID,
		Seq:       seq,
		EventID:   eventID,
		Kind:      string(e.EventKind()),
		EntityID:  e.EntityID(),
		Payload:   payload,
		CreatedAt: time.Now().UTC(),
	}
	if err := s.outbox.Insert(ctx, row); err != nil {
		s.lg.Warn("outbox insert failed",
			loggateway.Str("kind", string(e.EventKind())),
			loggateway.SessionID(sessionID),
			loggateway.Err(err))
		return ""
	}
	return rowID
}

// marshalV2EventEnvelope builds the same WS frame shape as WSV2Subscriber.
func marshalV2EventEnvelope(e biz.Event, eventID string) ([]byte, error) {
	envelope := map[string]any{
		"type":       "v2_event",
		"kind":       string(e.EventKind()),
		"session_id": e.SpiritSessionID(),
		"event_id":   eventID,
		"payload":    e,
	}
	return json.Marshal(envelope)
}

// persistLoop consumes persistChan with retry + dead-letter.
func (s *Sequencer) persistLoop() {
	defer s.persistWG.Done()
	for item := range s.persistChan {
		if item.flushCh != nil {
			close(item.flushCh)
			continue
		}
		s.persistWithRetry(item.event)
	}
}

func (s *Sequencer) persistWithRetry(e biz.Event) {
	var lastErr error
	for attempt := 0; attempt < s.persistMaxRetries; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_, err := persistAction(ctx, s.repoSet, e)
		cancel()
		if err == nil {
			return
		}
		lastErr = err
		// Exponential backoff: 1x, 2x, 4x, 8x, 16x.
		// Y7: no sleep after the final attempt — the event is about to be
		// dead-lettered, so the extra backoff only delays the failure signal.
		if attempt < s.persistMaxRetries-1 {
			time.Sleep(s.persistBackoff * time.Duration(1<<attempt))
		}
	}
	s.pushDeadLetterThrottledLog(e, lastErr)
	s.pushDeadLetter(e)
}

// pushDeadLetterThrottledLog emits the retry-exhaustion Error at most once
// per deadLetterErrLogInterval. Dead-lettering itself (pushDeadLetter) is
// never throttled — only the log line is.
func (s *Sequencer) pushDeadLetterThrottledLog(e biz.Event, lastErr error) {
	ok, suppressed := s.deadLetterErrThrottle.Allow()
	if !ok {
		return
	}
	fields := []loggateway.Field{loggateway.Str("kind", string(e.EventKind())), loggateway.Err(lastErr)}
	if suppressed > 0 {
		fields = append(fields, loggateway.Int("suppressed", suppressed))
	}
	s.lg.Error("persist exhausted retries, sending to dead-letter", fields...)
}

// Close performs graceful shutdown: close publishQueue → drain publishLoop →
// close persistChan → drain persistLoop → stop replay worker. Idempotent.
func (s *Sequencer) Close() error {
	s.closeMu.Lock()
	defer s.closeMu.Unlock()
	if !s.closed.CompareAndSwap(false, true) {
		return nil
	}
	// Y6: wait for in-flight Publish/Flush senders before closing the channel.
	s.pubMu.Lock()
	close(s.publishQueue)
	s.pubMu.Unlock()
	s.publishWG.Wait()
	close(s.persistChan)
	s.persistWG.Wait()
	if s.replayStarted {
		close(s.replayDone)
		s.replayWG.Wait()
	}
	return nil
}

// DeadLetterCount returns the number of events that exhausted retries.
func (s *Sequencer) DeadLetterCount() int { return s.deadLetter.Len() }

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
