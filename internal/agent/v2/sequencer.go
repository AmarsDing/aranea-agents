package v2

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
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
//
// P1-1: 持久化职责（persistChan/retry/deadLetter/replay）已拆分为
// persistWorker 子管理器（sequencer_persist.go + deadletter_replay.go），
// 日志限流聚合为 sequencerThrottles；本 struct 仅保留发布路径与生命周期字段。
type Sequencer struct {
	repoSet     RepoSet
	outbox      biz.EventDeliveryOutboxRepo // B-06: durable critical-event outbox (optional)
	bus         EventBus
	lg          loggateway.Logger
	seqAssigner SeqAssigner // shared with Projector (v2-local defaultSeqAssigner; breaks agent→v2 cycle)

	publishQueue       chan publishTask
	publishWG          sync.WaitGroup
	deltaBatchInterval time.Duration

	// persist 是持久化子管理器（P1-1）：persistChan/retry/deadLetter/replay。
	persist *persistWorker
	// throttles 聚合 5 个失败链日志限流器（行为从不被限流，仅日志）。
	throttles *sequencerThrottles

	// invariant 是 P1-2 事件溯源不变量开发态断言器，仅在
	// ARANEA_ORCH_INVARIANT=1 时非 nil（生产零开销）。
	invariant *invariantChecker

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
	// terminalFence is the optional FR-10 replay fence for
	// complete_task_terminal dead-letters (nil = legacy blind-apply).
	terminalFence DeadLetterTerminalFence
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

	slg := lg.With(loggateway.Domain("sequencer_v2"))
	throttles := newSequencerThrottles()
	s := &Sequencer{
		repoSet:               rs,
		outbox:                cfg.outbox,
		bus:                   bus,
		lg:                    slg,
		seqAssigner:           NewDefaultSeqAssigner(),
		publishQueue:          make(chan publishTask, cfg.publishBuffer),
		deltaBatchInterval:    cfg.deltaBatchInterval,
		persist:               newPersistWorker(rs, slg, throttles, cfg),
		throttles:             throttles,
		persistEnqueueTimeout: cfg.persistEnqueueTimeout,
	}
	if invariantCheckEnabled() {
		s.invariant = newInvariantChecker(slg)
		slg.Info("orchestration invariant assertion enabled (dev mode, log-only)",
			loggateway.Str("env", envOrchInvariant))
	}

	s.publishWG.Add(1)
	// 红线 #13：管道 worker 必须走 safego。裸 goroutine 一旦 panic，事件管道
	// 会静默死亡（事件堆积至 buffer 满后所有发布者阻塞在 ctx.Done）；
	// safego 提供 recover + PanicHook 告警。
	safego.GoBackground("sequencer-v2-publish", s.publishLoop)
	s.persist.start(cfg.disableReplayLoop)

	// AS-EVT-01 (2026-10-19 review fix): register as the bus-level critical
	// delegate so critical events published DIRECTLY to the bus by
	// non-Sequencer producers (service.PublishRunStatusFull, the session status
	// publisher, pause/resume gateways) are routed through this Sequencer's
	// publish pipeline — they receive a delivery Seq and a durable
	// event_delivery_outbox row like every other critical event, instead of a
	// live-only BlockUpTo with no replay coverage. The Sequencer's own fan-out
	// bypasses the hook via fanoutBus (V2Bus.Fanout), so no publish recursion.
	// Last registration wins when several Sequencers share a bus (test setups).
	if setter, ok := bus.(interface{ SetCriticalDelegate(event.CriticalDelegate) }); ok {
		if _, canFanout := bus.(fanoutBus); canFanout {
			setter.SetCriticalDelegate(s.PublishCritical)
		} else {
			// REG-1 (final-review 2026-10-19): on a bus that implements
			// SetCriticalDelegate but NOT fanoutBus, registering would make
			// s.fanout fall back to s.bus.Publish and re-enter this delegate for
			// every critical event — an unbounded self-enqueue livelock. Only
			// V2Bus implements both today; leave the delegate unset so those
			// events keep plain BlockUpTo semantics instead of livelocking.
			slg.Warn("critical delegate not registered: bus lacks Fanout; terminal frames stay BlockUpTo-only")
		}
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
		// R1: the Error is throttled (per-event under a stalled pipe); the
		// dead-letter push is not.
		if ok, suppressed := s.throttles.enqueueTimeoutErr.Allow(); ok {
			s.lg.Error("persist event enqueue timeout, sending to dead-letter",
				stallFields(suppressed,
					loggateway.Str("kind", string(e.EventKind())), loggateway.Err(enqueueCtx.Err()))...)
		}
		s.persist.pushDeadLetter(e)
	}
}

// shouldPersist returns false for streaming chunks and ephemeral system events
// (only bus-published; no entity Upsert).
func (s *Sequencer) shouldPersist(e biz.Event) bool {
	switch e.(type) {
	case *biz.StepStreamingEvent, *biz.SystemNoticeEvent, *biz.HeartbeatEvent, *biz.RunStatusEvent:
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
		case s.persist.ch <- persistItem{flushCh: persistFlushCh}:
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
	if s.invariant != nil {
		s.invariant.check(ev)
	}
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
// Best-effort WBPF is an INTENTIONAL design decision, not an accident
// (P2-1, 2026-10-19): strict WBPF (block publish until persist confirmed)
// was the legacy mode and was retired by ADR-02 — hiding a completed turn
// from the UI for the duration of a DB outage is strictly worse UX than a
// frontend that may momentarily lead the DB. The residual consistency window
// (all persist retries fail AND the client refreshes during the outage) is
// covered by an explicit four-layer recovery stack, deepest first:
//  1. Async persist retry: 5 exponential attempts (~1.5s backoff budget
//     — 100+200+400+800ms sleeps, no sleep after the final attempt;
//     excludes per-attempt execution time) → dead-letter (memory ring +
//     durable event_dead_letter with replay worker).
//  2. session_runs: written synchronously in the turn defer chain, fully
//     outside the event pipeline — the authoritative terminal marker for
//     REST reads.
//  3. Durable outbox: insertCriticalOutbox (via publishCritical below) runs
//     after the sync persist attempt on its OWN fresh 2s ctx, decoupled from
//     the task-level 5s ctx that the pre-WBPF drain may already have
//     exhausted — so the terminal frame lands in event_delivery_outbox even
//     on persist failure → reconnect replay (ListAfter) + P1-3 sweeper.
//  4. Frontend watchdog (P1-2): 90s of run-event silence while sending →
//     forced REST reconcile (refreshRunStatusForced); the UI self-heals from
//     the session_runs truth in layer 2.
//
// Do NOT "strictify" this path without revisiting ADR-02.
//
// Non-terminal events (created/updated/streaming) keep the original async
// persist + sync publish flow for low latency.
func (s *Sequencer) processTask(task publishTask) {
	if s.invariant != nil {
		s.invariant.check(task.event)
	}
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
			// L1: blocking-with-timeout enqueue of the flush marker. The prior
			// non-blocking `default:` skipped the drain exactly when the drain
			// mattered most (persistLoop backed up), letting a terminal sync
			// persist overtake the queued async persists of earlier events.
			select {
			case s.persist.ch <- persistItem{flushCh: drainCh}:
				select {
				case <-drainCh:
				case <-time.After(5 * time.Second):
					s.lg.Warn("terminal event persist-drain timed out, proceeding with sync persist",
						loggateway.Str("kind", string(task.event.EventKind())))
				}
			case <-time.After(2 * time.Second):
				// persistLoop still backed up after the grace window: proceed
				// with the best-effort sync persist (ordering hazard accepted,
				// logged loudly). Queued items retain async retry semantics.
				s.lg.Warn("persist channel full after drain-wait timeout, skipping pre-terminal drain",
					loggateway.Str("kind", string(task.event.EventKind())))
			}
			persistCtx, persistCancel := context.WithTimeout(context.Background(), 2*time.Second)
			_, err := persistAction(persistCtx, s.repoSet, task.event)
			persistCancel()
			if err == nil {
				// Sync persist succeeded — durable outbox then publish (B-06).
				s.publishCritical(task.event)
				return
			}
			// Sync persist failed — log and fall through to async retry + publish.
			s.lg.Warn("terminal event sync persist failed, falling back to async",
				loggateway.Str("kind", string(task.event.EventKind())),
				loggateway.Err(err))
		}
		// Async persist (non-terminal or terminal fallback).
		select {
		case s.persist.ch <- persistItem{event: task.event}:
		default:
			// Persist channel full: log + drop to dead-letter.
			// R2: the Warn is throttled (per-event while the persist worker is
			// stuck); the dead-letter push is not.
			if ok, suppressed := s.throttles.persistFullWarn.Allow(); ok {
				s.lg.Warn("persist channel full, event dropped to dead-letter",
					stallFields(suppressed,
						loggateway.Str("kind", string(task.event.EventKind())))...)
			}
			s.persist.pushDeadLetter(task.event)
		}
	}
	if biz.IsCriticalDeliveryEvent(task.event) {
		s.publishCritical(task.event)
		return
	}
	// Sync bus publish.
	s.bus.Publish(ctx, task.event)
}

// fanoutBus is implemented by *event.V2Bus: Fanout skips the critical
// delegate (which points back at this Sequencer) so critical events handled
// here are fanned out exactly once instead of re-entering the delegate.
type fanoutBus interface {
	Fanout(ctx context.Context, e biz.Event)
}

// fanout publishes e to the bus without re-entering the critical delegate.
//
// Fallback invariant (REG-1): the s.bus.Publish path below is safe ONLY because
// NewSequencer refuses to register the critical delegate when the bus lacks
// fanoutBus — keep those two checks coupled. Registering the delegate on such
// a bus would re-enter PublishCritical here on every critical event (livelock).
func (s *Sequencer) fanout(ctx context.Context, e biz.Event) {
	if fb, ok := s.bus.(fanoutBus); ok {
		fb.Fanout(ctx, e)
		return
	}
	s.bus.Publish(ctx, e)
}

// PublishCritical is the V2Bus critical delegate (registered in NewSequencer):
// it routes critical events that producers published DIRECTLY to the bus —
// service-level terminal/reset-required run_status, session_status_changed,
// pause/resume frames — through the Sequencer's FIFO publish queue so they
// receive a delivery Seq and a durable outbox row like every other critical
// event (AS-EVT-01). Non-critical events never reach this path.
//
// Backpressure semantics: unlike ephemeral streaming/heartbeat frames (which
// may drop on ctx cancel), a critical enqueue blocks up to
// persistEnqueueTimeout on a cancel-detached ctx. If the queue is still full
// past that window the event is delivered synchronously via the outbox
// fallback (publishCriticalDirect) — terminal/reset-required frames are never
// silently dropped; the outbox row is the floor, not the queue.
func (s *Sequencer) PublishCritical(ctx context.Context, e biz.Event) {
	if e == nil {
		return
	}
	// Y6: same closed-check + send critical section as Publish.
	s.pubMu.RLock()
	defer s.pubMu.RUnlock()
	if s.closed.Load() {
		// Shutdown in flight: the queue path is gone; deliver synchronously so
		// a terminal frame published during Close still lands (best-effort).
		s.publishCriticalDirect(e)
		return
	}
	enqueueCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), s.persistEnqueueTimeout)
	defer cancel()
	select {
	case s.publishQueue <- publishTask{event: e, persist: false, enqueuedAt: time.Now()}:
	case <-enqueueCtx.Done():
		// Queue stalled past the grace window: fall back to synchronous
		// outbox + fan-out so the critical frame is never lost.
		if ok, suppressed := s.throttles.enqueueTimeoutErr.Allow(); ok {
			s.lg.Error("critical event enqueue timeout, delivering via direct outbox fallback",
				stallFields(suppressed,
					loggateway.Str("kind", string(e.EventKind())), loggateway.Err(enqueueCtx.Err()))...)
		}
		s.publishCriticalDirect(e)
	}
}

// publishCriticalDirect delivers a critical event synchronously on the
// caller's goroutine (outbox row → fan-out → MarkPublished), bypassing the
// publish queue. Used only when the queue is unavailable (Sequencer closed
// or saturated past the enqueue timeout).
func (s *Sequencer) publishCriticalDirect(e biz.Event) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	outboxID := s.insertCriticalOutbox(ctx, e)
	s.fanout(ctx, e)
	if outboxID != "" && s.outbox != nil {
		if err := s.outbox.MarkPublished(ctx, outboxID, time.Now().UTC()); err != nil {
			s.lg.Warn("outbox mark published failed",
				loggateway.Str("kind", string(e.EventKind())),
				loggateway.Err(err))
		}
	}
}

// publishCritical writes the durable outbox row (best-effort) then fans out.
//
// Recovery-stack fix (P1 review, 2026-10-19): the outbox Insert, fan-out and
// MarkPublished deliberately run on an independent fresh 2s ctx instead of
// processTask's 5s task ctx. That budget is routinely consumed before this
// point by the pre-WBPF drain wait (up to ~2s enqueue grace + ~5s drain
// wait), so an inherited ctx is often already expired exactly when the DB is
// backing up — the failure mode the outbox layer exists to cover. A fresh
// short ctx keeps the layer-3 outbox row writable without letting a stalled
// DB stall the publish worker for longer.
func (s *Sequencer) publishCritical(e biz.Event) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	outboxID := s.insertCriticalOutbox(ctx, e)
	s.fanout(ctx, e)
	if outboxID != "" && s.outbox != nil {
		if err := s.outbox.MarkPublished(ctx, outboxID, time.Now().UTC()); err != nil {
			s.lg.Warn("outbox mark published failed",
				loggateway.Str("kind", string(e.EventKind())),
				loggateway.Err(err))
		}
	}
}

// insertCriticalOutbox assigns a delivery seq when missing, inserts an outbox
// row, and returns its id. Failures are logged and ignored so publish is never
// fail-closed on outbox errors.
//
// The delivery seq ALWAYS comes from the shared per-session SeqAssigner
// counter — never from the event's embedded entity Seq (domain data, e.g. the
// turn counter 1,2,3…). The outbox's UNIQUE(session_id, seq) index and
// ListAfter(seq > cursor) replay both require one monotonic counter space per
// session; mixing turn seqs into it collided/skipped terminal frames on
// reconnect (H1, 2026-10-18 review).
func (s *Sequencer) insertCriticalOutbox(ctx context.Context, e biz.Event) string {
	if s.outbox == nil || e == nil || !biz.IsCriticalDeliveryEvent(e) {
		return ""
	}
	sessionID := e.SpiritSessionID()
	if sessionID == "" {
		return ""
	}
	de, ok := e.(biz.DeliverySeqEvent)
	if !ok {
		s.lg.Warn("critical event lacks delivery-seq carrier, outbox skipped",
			loggateway.Str("kind", string(e.EventKind())))
		return ""
	}
	seq := de.DeliverySeqValue()
	if seq <= 0 {
		if s.seqAssigner == nil {
			return ""
		}
		seq = s.seqAssigner.NextSeq(sessionID)
		de.SetDeliverySeqValue(seq)
	}
	// buildRow 按当前 seq 产出 outbox 行（payload 内嵌 eventID，换 seq 必须
	// 重新 marshal——否则 durable 行与 live 帧的 event_id 分叉，回放去重失真）。
	buildRow := func() (biz.EventDeliveryOutboxRow, error) {
		eventID := biz.DeliveryEventID(e, seq)
		payload, err := marshalV2EventEnvelope(e, eventID)
		if err != nil {
			return biz.EventDeliveryOutboxRow{}, err
		}
		return biz.EventDeliveryOutboxRow{
			ID:        uuid.NewString(),
			SessionID: sessionID,
			Seq:       seq,
			EventID:   eventID,
			Kind:      string(e.EventKind()),
			EntityID:  e.EntityID(),
			Payload:   payload,
			CreatedAt: time.Now().UTC(),
		}, nil
	}
	row, err := buildRow()
	if err != nil {
		s.lg.Warn("outbox marshal failed",
			loggateway.Str("kind", string(e.EventKind())),
			loggateway.Err(err))
		return ""
	}
	if err := s.outbox.Insert(ctx, row); err != nil {
		// FR-9：(session_id, seq) 冲突 = 共享 SeqAssigner 落后于持久水位
		// （重启恢复靠 FR-9 max(steps,tasks,outbox) 折入闭合主窗口；残余
		// DeltaSeq 尾部消耗无持久水位可被一次重取覆盖）。重取 seq 重试
		// 一次并 Warn；仍失败回落既有 best-effort 跳过。
		if errors.Is(err, biz.ErrEventOutboxSeqConflict) && s.seqAssigner != nil {
			seq = s.seqAssigner.NextSeq(sessionID)
			de.SetDeliverySeqValue(seq)
			if retryRow, buildErr := buildRow(); buildErr == nil {
				if retryErr := s.outbox.Insert(ctx, retryRow); retryErr == nil {
					s.lg.Warn("outbox seq conflict recovered by retry",
						loggateway.Str("kind", string(e.EventKind())),
						loggateway.SessionID(sessionID),
						loggateway.Int64("retried_seq", seq),
						loggateway.Err(err))
					return retryRow.ID
				} else {
					err = retryErr
				}
			}
		}
		// R4: throttled — fires per critical event while the outbox table is
		// down. The outbox skip itself is unchanged (best-effort).
		if ok, suppressed := s.throttles.outboxInsertWarn.Allow(); ok {
			s.lg.Warn("outbox insert failed",
				stallFields(suppressed,
					loggateway.Str("kind", string(e.EventKind())),
					loggateway.SessionID(sessionID),
					loggateway.Err(err))...)
		}
		return ""
	}
	return row.ID
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

// Close performs graceful shutdown: close publishQueue → drain publishLoop →
// close persist channel → drain persistLoop → stop replay worker. Idempotent.
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
	s.persist.close()
	return nil
}

// DeadLetterCount returns the number of events that exhausted retries.
func (s *Sequencer) DeadLetterCount() int { return s.persist.ring.Len() }

// InvariantViolationCount returns the number of event-lineage invariant
// violations detected so far (P1-2). Returns 0 when the dev-mode assertion
// is disabled (ARANEA_ORCH_INVARIANT unset).
func (s *Sequencer) InvariantViolationCount() int64 {
	if s.invariant == nil {
		return 0
	}
	return s.invariant.violations()
}
