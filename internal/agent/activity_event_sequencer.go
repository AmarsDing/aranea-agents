package agent

import (
	"context"
	"errors"
	"sync"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
)

// defaultPublishBufferSize is the buffer size of the shared publish queue.
// When full, publish blocks (backpressure) which propagates to the LLM:
// queue full → OnTextDelta blocks → stream_consumer blocks → LLM pauses.
const defaultPublishBufferSize = 256

// defaultPersistBufferSize is the buffer size of the shared persist channel.
const defaultPersistBufferSize = 256

// defaultDeltaBatchInterval is the maximum time window during which consecutive
// streaming events for the same field are coalesced into a single event.
const defaultDeltaBatchInterval = 16 * time.Millisecond

// persist retry configuration (unchanged from v1)
const (
	persistMaxRetries       = 5
	persistInitialBackoffMs = 100
	persistBackoffFactor    = 2
)

// deadLetterCapacity is the maximum number of failed-persist activities
// retained in the dead-letter buffer.
const deadLetterCapacity = 512

// errSequencerClosed is returned when publishing to a closed sequencer.
var errSequencerClosed = errors.New("activity event sequencer closed")

// activityEventSequencer (v2): single publish worker architecture.
//
// The v2 design replaces per-activity channels (v1) with a single shared
// publish queue + one worker goroutine. The publish worker processes tasks
// in strict FIFO order, which guarantees:
//   - Cross-activity order: tasks are published in the exact order they were
//     enqueued, which (since seq is pre-allocated at On* entry) matches the
//     projector business order. The v1 per-activity channels had goroutine
//     scheduling races that allowed reply events to be published before
//     thinking events.
//   - Single-activity FIFO: tasks for the same activity are naturally ordered
//     because the On* methods are serialized under p.mu.
//   - I/O offload: publish/persist still happen in worker goroutines, so
//     OnTextDelta does not block on WS or DB I/O (B-04 fix preserved).
//
// Design rationale:
//   - Single publish queue: no cross-goroutine channel ordering issues
//   - One worker: serializes eventBus.Publish calls → WS subscriber FIFO
//   - Separate persist worker: DB I/O parallelism (unchanged from v1)
type activityEventSequencer struct {
	eventBus     biz.ActivityEventBus
	activityRepo biz.ActivityWriter
	lg           loggateway.Logger

	// publishQueue: single shared FIFO queue feeding the publish worker.
	// All On* methods enqueue tasks here (under p.mu for seq ordering).
	publishQueue chan publishTask
	publishWg    sync.WaitGroup

	// publishWorkerStarted guards the lazy single-start of the publish
	// worker inside startPublishWorker (called from publish()).
	publishWorkerStarted bool

	// persistChan: feeds the single persist worker goroutine.
	persistChan chan persistItem
	persistWg   sync.WaitGroup

	// Lifecycle
	mu     sync.Mutex
	closed bool
	done   chan struct{}

	// deltaBatchInterval: streaming events coalescing window
	deltaBatchInterval time.Duration

	// Retry parameters
	persistMaxRetries       int
	persistInitialBackoffMs int
	persistBackoffFactor    int

	// deadLetter ring buffer
	deadLetterMu sync.Mutex
	deadLetter   []biz.Activity
}

// persistItem is a single activity to persist
type persistItem struct {
	activityID string
	activity   biz.Activity
}

// publishTask represents a single ActivityEvent to publish and optionally persist.
type publishTask struct {
	event    biz.ActivityEvent
	persist  bool
	activity biz.Activity
}

// newActivityEventSequencer creates a new v2 sequencer.
// The publish worker is started lazily on the first publish() call, so
// that callers can safely configure fields like deltaBatchInterval between
// construction and the first publish. The persist worker is started lazily
// by SetActivityRepo.
func newActivityEventSequencer(eventBus biz.ActivityEventBus, lg loggateway.Logger) *activityEventSequencer {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &activityEventSequencer{
		eventBus:                eventBus,
		lg:                      lg,
		done:                    make(chan struct{}),
		deltaBatchInterval:      defaultDeltaBatchInterval,
		persistMaxRetries:       persistMaxRetries,
		persistInitialBackoffMs: persistInitialBackoffMs,
		persistBackoffFactor:    persistBackoffFactor,
		publishQueue:            make(chan publishTask, defaultPublishBufferSize),
	}
}

// publish enqueues a task to the publish queue. Blocks if queue is full
// (backpressure), or returns errSequencerClosed if sequencer is closed.
//
// The activityID parameter is preserved for API compatibility with v1 callers
// and logging, but is not used for routing in v2 (single shared queue).
//
// The publish worker is started lazily on the first call so that tests can
// configure fields like deltaBatchInterval between construction and the
// first publish. startPublishWorker() is safe under concurrent invocation.
func (s *activityEventSequencer) publish(ctx context.Context, activityID string, task publishTask) error {
	s.startPublishWorker()
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return errSequencerClosed
	}
	s.mu.Unlock()

	select {
	case s.publishQueue <- task:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-s.done:
		return errSequencerClosed
	}
}

// startPublishWorker lazily starts the single publish worker on the first
// call. Subsequent calls are no-ops. Uses s.mu for the flag check + flip,
// which serializes against Close()'s flag check.
func (s *activityEventSequencer) startPublishWorker() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.publishWorkerStarted {
		return
	}
	s.publishWorkerStarted = true
	s.publishWg.Add(1)
	safego.GoBackground("activity_publish_worker", func() {
		s.runPublishWorker()
	})
}

// runPublishWorker is the single goroutine that processes all publish tasks
// in FIFO order. It is started lazily on first SetActivityRepo call.
//
// Streaming events are batched within deltaBatchInterval to reduce event
// frequency (≤60fps to frontend). The batch window is preserved from v1.
func (s *activityEventSequencer) runPublishWorker() {
	defer s.publishWg.Done()

	batchInterval := s.deltaBatchInterval
	if batchInterval <= 0 {
		batchInterval = defaultDeltaBatchInterval
	}

	timer := time.NewTimer(batchInterval)
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
	defer timer.Stop()

	var pending *publishTask

	flush := func() {
		if pending == nil {
			return
		}
		s.processTask(*pending)
		pending = nil
	}
	defer flush()

	resetTimer := func() {
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		timer.Reset(batchInterval)
	}

	mergeDelta := func(dst *publishTask, src publishTask) {
		dst.event.DeltaChunk += src.event.DeltaChunk
	}

	for {
		select {
		case task, ok := <-s.publishQueue:
			if !ok {
				// Queue closed by Close(); drain remaining pending and exit
				return
			}
			if task.event.Event != biz.ActivityEventStreaming {
				// Non-streaming events (created/completed/etc.) must be published
				// immediately so terminal events are not delayed by the batch window.
				flush()
				s.processTask(task)
				continue
			}
			if pending != nil && s.canMergeDeltas(*pending, task) {
				mergeDelta(pending, task)
				resetTimer()
				continue
			}
			flush()
			pending = &task
			resetTimer()

		case <-timer.C:
			flush()

		case <-s.done:
			// Sequencer is closing; drain queue and exit
			for {
				select {
				case task, ok := <-s.publishQueue:
					if !ok {
						return
					}
					if task.event.Event != biz.ActivityEventStreaming {
						flush()
						s.processTask(task)
						continue
					}
					if pending != nil && s.canMergeDeltas(*pending, task) {
						mergeDelta(pending, task)
						continue
					}
					flush()
					pending = &task
				default:
					return
				}
			}
		}
	}
}

// canMergeDeltas reports whether two consecutive streaming events can be
// coalesced into a single event.
func (s *activityEventSequencer) canMergeDeltas(a, b publishTask) bool {
	if a.event.Event != biz.ActivityEventStreaming || b.event.Event != biz.ActivityEventStreaming {
		return false
	}
	if a.event.DeltaField == "" || b.event.DeltaField == "" || a.event.DeltaField != b.event.DeltaField {
		return false
	}
	return true
}

// processTask persists the activity (fire-and-forget) and publishes the
// ActivityEvent synchronously. Called from the single publish worker —
// serializes all eventBus.Publish calls.
func (s *activityEventSequencer) processTask(task publishTask) {
	if task.persist && s.activityRepo != nil {
		item := persistItem{activityID: task.activity.ID, activity: task.activity}
		select {
		case s.persistChan <- item:
			// enqueued for async persist
		default:
			// Channel full: fall back to synchronous persist
			s.persistWithRetry(task.activity.ID, task.activity, true)
		}
	}
	if s.eventBus != nil {
		s.eventBus.Publish(context.Background(), task.event)
	}
}

// persistWithRetry calls UpsertActivity with exponential backoff retry.
func (s *activityEventSequencer) persistWithRetry(activityID string, a biz.Activity, syncFallback bool) {
	backoff := s.persistInitialBackoffMs
	path := "worker"
	if syncFallback {
		path = "sync_fallback"
	}
	for attempt := 0; attempt <= s.persistMaxRetries; attempt++ {
		if _, err := s.activityRepo.UpsertActivity(context.Background(), a); err == nil {
			return
		} else if attempt == s.persistMaxRetries {
			s.lg.Warn("activity persist failed after retries; pushed to dead-letter buffer",
				loggateway.StepID("agent.activity_sequencer.persist"),
				loggateway.Str("activity_id", activityID),
				loggateway.Str("kind", string(a.Kind)),
				loggateway.Str("status", string(a.Status)),
				loggateway.Str("path", path),
				loggateway.Int("attempts", attempt+1),
				loggateway.Err(err))
			s.pushDeadLetter(a)
			return
		}
		select {
		case <-s.done:
			s.lg.Warn("activity persist aborted during shutdown; pushed to dead-letter buffer",
				loggateway.StepID("agent.activity_sequencer.persist"),
				loggateway.Str("activity_id", activityID),
				loggateway.Str("path", path),
				loggateway.Int("attempt", attempt+1))
			s.pushDeadLetter(a)
			return
		case <-time.After(time.Duration(backoff) * time.Millisecond):
		}
		backoff *= s.persistBackoffFactor
	}
}

// pushDeadLetter appends a failed-persist activity to the dead-letter buffer.
func (s *activityEventSequencer) pushDeadLetter(a biz.Activity) {
	s.deadLetterMu.Lock()
	defer s.deadLetterMu.Unlock()
	for i := range s.deadLetter {
		if s.deadLetter[i].ID == a.ID {
			s.deadLetter[i] = a
			return
		}
	}
	if len(s.deadLetter) >= deadLetterCapacity {
		s.deadLetter = append(s.deadLetter[:0], s.deadLetter[1:]...)
	}
	s.deadLetter = append(s.deadLetter, a)
}

// ListDeadLetterActivities returns a snapshot of dead-letter activities.
func (s *activityEventSequencer) ListDeadLetterActivities(sessionID string) []biz.Activity {
	s.deadLetterMu.Lock()
	defer s.deadLetterMu.Unlock()
	if len(s.deadLetter) == 0 {
		return nil
	}
	out := make([]biz.Activity, 0, len(s.deadLetter))
	for _, a := range s.deadLetter {
		if sessionID == "" || a.SessionID == sessionID {
			out = append(out, a)
		}
	}
	return out
}

// SetActivityRepo sets the activity repository and starts the persist worker.
// The persist worker is started lazily here (only when a non-nil repo is
// configured). The publish worker is started in newActivityEventSequencer
// and does not depend on persistence.
//
// Once set, processTask will fire-and-forget persist activities via a single
// worker goroutine, preserving FIFO order without blocking WS publish.
func (s *activityEventSequencer) SetActivityRepo(repo biz.ActivityWriter) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.activityRepo = repo
	if repo != nil && s.persistChan == nil {
		s.persistChan = make(chan persistItem, defaultPersistBufferSize)
		s.persistWg.Add(1)
		safego.GoBackground("activity_persist_worker", func() {
			s.runPersistWorker()
		})
	}
}

// runPersistWorker is the single persist goroutine.
func (s *activityEventSequencer) runPersistWorker() {
	defer s.persistWg.Done()
	for item := range s.persistChan {
		s.persistWithRetry(item.activityID, item.activity, false)
	}
}

// Close closes the sequencer and waits for all workers to finish.
//
// Shutdown sequence:
//  1. Signal workers to drain and exit (close s.done)
//  2. Close publishQueue — signals the publish worker to exit after draining
//  3. Wait for the publish worker to finish (s.publishWg.Wait)
//  4. Close persistChan — signals the persist worker to exit
//  5. Wait for the persist worker to finish (s.persistWg.Wait)
//
// After Close returns, all queued events have been processed (published and
// persisted). Subsequent publish calls return errSequencerClosed.
//
// Close is idempotent and safe to call multiple times.
func (s *activityEventSequencer) Close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	close(s.done)
	s.mu.Unlock()

	// Close publish queue to signal publish worker to exit
	close(s.publishQueue)
	s.publishWg.Wait()

	// Close persist channel
	if s.persistChan != nil {
		close(s.persistChan)
		s.persistWg.Wait()
	}
}
