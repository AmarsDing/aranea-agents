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

	publishWG sync.WaitGroup
	persistWG sync.WaitGroup

	deltaBatchInterval time.Duration
	persistMaxRetries  int
	persistBackoff     time.Duration

	closed  atomic.Bool
	closeMu sync.Mutex
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
	publishBuffer      int
	persistBuffer      int
	deltaBatchInterval time.Duration
	persistMaxRetries  int
	persistBackoff     time.Duration
	deadLetterCapacity int
	outbox             biz.EventDeliveryOutboxRepo
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

// WithEventOutbox injects the durable critical-event outbox (B-06).
// When set, critical events are written to the outbox after entity persist
// (WBPF path) and before bus fan-out so WS reconnect can replay by last_event_id.
func WithEventOutbox(repo biz.EventDeliveryOutboxRepo) Option {
	return func(c *config) { c.outbox = repo }
}

// NewSequencer constructs a Sequencer and starts its publish + persist workers.
func NewSequencer(rs RepoSet, bus EventBus, lg loggateway.Logger, opts ...Option) *Sequencer {
	cfg := config{
		publishBuffer:      defaultPublishBufferSize,
		persistBuffer:      defaultPersistBufferSize,
		deltaBatchInterval: defaultDeltaBatchInterval,
		persistMaxRetries:  defaultPersistMaxRetries,
		persistBackoff:     defaultPersistBackoff,
		deadLetterCapacity: defaultDeadLetterCapacity,
	}
	for _, o := range opts {
		o(&cfg)
	}

	s := &Sequencer{
		repoSet:            rs,
		outbox:             cfg.outbox,
		bus:                bus,
		lg:                 lg.With(loggateway.Domain("sequencer_v2")),
		seqAssigner:        NewDefaultSeqAssigner(),
		publishQueue:       make(chan publishTask, cfg.publishBuffer),
		persistChan:        make(chan persistItem, cfg.persistBuffer),
		deadLetter:         newDeadLetterRing(cfg.deadLetterCapacity),
		deltaBatchInterval: cfg.deltaBatchInterval,
		persistMaxRetries:  cfg.persistMaxRetries,
		persistBackoff:     cfg.persistBackoff,
	}

	s.publishWG.Add(1)
	go s.publishLoop()
	s.persistWG.Add(1)
	go s.persistLoop()
	return s
}

// SeqAssigner exposes the shared SeqAssigner so Projector can pre-allocate Seq
// for turn-level events before publishing.
func (s *Sequencer) SeqAssigner() SeqAssigner { return s.seqAssigner }

// Publish enqueues an event for FIFO processing.
// Safe for concurrent use.
func (s *Sequencer) Publish(ctx context.Context, e biz.Event) {
	if s.closed.Load() {
		s.lg.Warn("sequencer closed, event dropped", loggateway.Str("kind", string(e.EventKind())))
		return
	}
	persist := s.shouldPersist(e)
	select {
	case s.publishQueue <- publishTask{event: e, persist: persist, enqueuedAt: time.Now()}:
	case <-ctx.Done():
		s.lg.Warn("publish ctx canceled before enqueue",
			loggateway.Str("kind", string(e.EventKind())), loggateway.Err(ctx.Err()))
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
				// Start a new pending streaming with timer
				pendingStreaming = ev
				pendingDone = make(chan struct{})
				pendingTimer = time.AfterFunc(s.deltaBatchInterval, func() {
					close(pendingDone)
				})
				_ = pendingTimer
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
			s.deadLetter.Push(task.event)
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
		// Exponential backoff: 1x, 2x, 4x, 8x, 16x
		time.Sleep(s.persistBackoff * time.Duration(1<<attempt))
	}
	s.lg.Error("persist exhausted retries, sending to dead-letter",
		loggateway.Str("kind", string(e.EventKind())), loggateway.Err(lastErr))
	s.deadLetter.Push(e)
}

// Close performs graceful shutdown: close publishQueue → drain publishLoop →
// close persistChan → drain persistLoop. Idempotent.
func (s *Sequencer) Close() error {
	s.closeMu.Lock()
	defer s.closeMu.Unlock()
	if !s.closed.CompareAndSwap(false, true) {
		return nil
	}
	close(s.publishQueue)
	s.publishWG.Wait()
	close(s.persistChan)
	s.persistWG.Wait()
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
		return // already in dead letter; skip
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
