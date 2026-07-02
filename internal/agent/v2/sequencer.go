package v2

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"aranea-agents/internal/agent"
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
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
	bus         EventBus
	lg          loggateway.Logger
	seqAssigner *agent.SeqAssigner // shared with Projector (Deviation 3: lives in package agent)

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
}

type persistItem struct {
	event biz.Event
}

type config struct {
	publishBuffer       int
	persistBuffer       int
	deltaBatchInterval  time.Duration
	persistMaxRetries   int
	persistBackoff      time.Duration
	deadLetterCapacity  int
}

// Option configures a Sequencer.
type Option func(*config)

func WithPublishBuffer(n int) Option                { return func(c *config) { c.publishBuffer = n } }
func WithPersistBuffer(n int) Option                { return func(c *config) { c.persistBuffer = n } }
func WithDeltaBatchInterval(d time.Duration) Option { return func(c *config) { c.deltaBatchInterval = d } }
func WithPersistMaxRetries(n int) Option            { return func(c *config) { c.persistMaxRetries = n } }
func WithPersistBackoff(d time.Duration) Option     { return func(c *config) { c.persistBackoff = d } }
func WithDeadLetterCapacity(n int) Option           { return func(c *config) { c.deadLetterCapacity = n } }

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
		bus:                bus,
		lg:                 lg.With(loggateway.Domain("sequencer_v2")),
		seqAssigner:        agent.NewSeqAssigner(),
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
func (s *Sequencer) SeqAssigner() *agent.SeqAssigner { return s.seqAssigner }

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

// shouldPersist returns false for streaming chunks (only bus-published).
func (s *Sequencer) shouldPersist(e biz.Event) bool {
	_, ok := e.(*biz.StepStreamingEvent)
	return !ok
}

// Flush blocks until all queued events are processed (publish + persist).
// Mainly for tests; production callers should rely on Close() for shutdown.
//
// Note: also waits deltaBatchInterval + small buffer so any pending streaming
// event held in publishLoop's local variable can be flushed by its timer.
func (s *Sequencer) Flush(ctx context.Context) error {
	done := make(chan struct{})
	go func() {
		// Wait until publishQueue empties.
		for len(s.publishQueue) > 0 {
			time.Sleep(time.Millisecond)
		}
		// Allow pending streaming timer to fire.
		time.Sleep(s.deltaBatchInterval + time.Millisecond*2)
		// Wait until persistChan empties.
		for len(s.persistChan) > 0 {
			time.Sleep(time.Millisecond)
		}
		close(done)
	}()
	select {
	case <-done:
		return nil
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

// processTask handles a non-mergeable event: persist (async) + bus publish (sync).
func (s *Sequencer) processTask(task publishTask) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// 1. Async persist (skip if streaming — but streaming never reaches here).
	if task.persist {
		select {
		case s.persistChan <- persistItem{event: task.event}:
		default:
			// Persist channel full: log + drop to dead-letter.
			s.lg.Warn("persist channel full, event dropped to dead-letter",
				loggateway.Str("kind", string(task.event.EventKind())))
			s.deadLetter.Push(task.event)
		}
	}
	// 2. Sync bus publish.
	s.bus.Publish(ctx, task.event)
}

// persistLoop consumes persistChan with retry + dead-letter.
func (s *Sequencer) persistLoop() {
	defer s.persistWG.Done()
	for item := range s.persistChan {
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
