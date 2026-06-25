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

// defaultChannelBufferSize is the per-activity channel buffer size.
// When full, publish blocks (backpressure) which propagates to the LLM:
// channel full → OnTextDelta blocks → stream_consumer blocks → LLM pauses.
const defaultChannelBufferSize = 64

// defaultPersistBufferSize is the buffer size of the shared persist channel.
// When full, persist falls back to synchronous mode (blocking the consumer)
// to avoid losing data. This only happens under extreme DB latency.
const defaultPersistBufferSize = 256

// defaultDeltaBatchInterval is the maximum time window during which consecutive
// streaming events for the same field are coalesced into a single event.
// This reduces frontend event frequency from "one per token" to
// "at most one per 60fps frame", eliminating UI jank without perceptible lag.
const defaultDeltaBatchInterval = 16 * time.Millisecond

// persist retry configuration (CS-B15: retry upper bound + exponential backoff).
// On transient DB errors (lock contention, brief unavailability), retrying
// avoids data loss that would force the frontend to backfill via API.
//
// Tuned to align with SQLite's busy_timeout=30000ms: total worst-case added
// latency is 100+200+400+800+1600 = 3100ms before giving up, which is well
// within the busy_timeout window and gives lock holders ample time to release.
const (
	persistMaxRetries       = 5
	persistInitialBackoffMs = 100
	persistBackoffFactor    = 2
)

// deadLetterCapacity is the maximum number of failed-persist activities
// retained in the dead-letter buffer. The buffer is a ring: when full, the
// oldest entry is evicted. Activities here could not be persisted after all
// retries — the WS stream still delivered them (fire-and-forget publish),
// so the buffer serves as a short-term compensation source for reconnect
// replay via ListDeadLetterActivities.
const deadLetterCapacity = 512

// errSequencerClosed is returned when publishing to a closed sequencer.
var errSequencerClosed = errors.New("activity event sequencer closed")

// activityEventSequencer guarantees per-activity FIFO event ordering.
//
// Each activity gets its own buffered channel and a dedicated consumer
// goroutine. Events for the same activity are strictly ordered
// (created → streaming → completed), while events for different activities
// can be processed concurrently.
//
// Design rationale:
//   - Per-activity channel: guarantees FIFO without a global lock
//   - Consumer goroutine: I/O (publish + persist) happens outside caller's
//     critical section, so the event loop is never blocked by I/O
//   - Backpressure: when channel is full, publish blocks, propagating
//     backpressure to the LLM stream consumer
//   - No per-activity channel close: avoids send-on-closed-channel races;
//     channels are drained and goroutines exit via the done signal on Close
//
// This fixes B-01 (start/delta ordering issue), B-04 (delta holds global
// lock blocking all tokens), and B-05 (async start races with sync delta).
type activityEventSequencer struct {
	mu       sync.Mutex
	channels map[string]chan publishTask
	eventBus biz.ActivityEventBus
	activityRepo       biz.ActivityWriter
	lg                 loggateway.Logger
	wg                 sync.WaitGroup // consumer goroutines
	persistWg          sync.WaitGroup // persist worker goroutine
	closed             bool
	done               chan struct{}
	deltaBatchInterval time.Duration

	// persistChan feeds the single persist worker goroutine. Using one
	// shared worker (instead of per-task goroutines) guarantees that
	// UpsertActivity calls for the same activity execute in FIFO order,
	// so a late "start" persist can never overwrite an earlier "done"
	// persist. The worker is started lazily when a repo is set.
	// persistChan is closed by Close() AFTER all consumers have exited,
	// ensuring no items are lost.
	persistChan chan persistItem

	// deadLetter is a short-term ring buffer holding activities whose
	// persist failed after all retries. The WS layer already delivered
	// them (fire-and-forget publish), so on reconnect the replay path
	// must merge ListActivities RPC results with this buffer to avoid
	// showing users a gap. The buffer is process-scoped (not persisted):
	// it survives WS reconnects but not process restarts.
	deadLetterMu sync.Mutex
	deadLetter   []biz.Activity
}

// persistItem is a single activity to persist, paired with its activity ID
// for logging.
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

// newActivityEventSequencer creates a new sequencer.
// The sequencer must be Closed when no longer needed to release goroutines.
func newActivityEventSequencer(eventBus biz.ActivityEventBus, lg loggateway.Logger) *activityEventSequencer {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &activityEventSequencer{
		channels:           make(map[string]chan publishTask),
		eventBus:           eventBus,
		lg:                 lg,
		done:               make(chan struct{}),
		deltaBatchInterval: defaultDeltaBatchInterval,
	}
}

// publish sends a task to the activity's channel.
//
// If the activity's channel doesn't exist, a new one is created along with
// a consumer goroutine. The call blocks until the task is enqueued, the
// context is cancelled, or the sequencer is closed.
//
// Backpressure: when the channel buffer is full, publish blocks. This
// propagates backpressure to the caller (e.g., OnTextDelta), which
// propagates to the stream consumer, which pauses the LLM stream.
func (s *activityEventSequencer) publish(ctx context.Context, activityID string, task publishTask) error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return errSequencerClosed
	}
	ch, ok := s.channels[activityID]
	if !ok {
		ch = make(chan publishTask, defaultChannelBufferSize)
		s.channels[activityID] = ch
		s.wg.Add(1)
		// safego.GoBackground (red line #13): consumer goroutines are
		// process-level (outlive any single request), exit via s.done
		// channel on Close. safego provides panic recovery + PanicHook.
		safego.GoBackground("activity_event_sequencer.consume", func() {
			s.consume(activityID, ch)
		})
	}
	s.mu.Unlock()

	select {
	case ch <- task:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-s.done:
		return errSequencerClosed
	}
}

// consume is the per-activity consumer goroutine.
//
// It reads tasks from the channel and processes them in FIFO order:
//  1. Persist the activity to the database (if persist=true)
//  2. Publish the ActivityEvent to the event bus
//
// Consecutive streaming events for the same field are batched within
// deltaBatchInterval to reduce frontend event frequency while preserving order.
//
// The goroutine exits when the channel is drained and closed, or when the
// done signal is received (on Close).
func (s *activityEventSequencer) consume(activityID string, ch <-chan publishTask) {
	defer s.wg.Done()

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
		s.processTask(activityID, *pending)
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
		case task, ok := <-ch:
			if !ok {
				return
			}
			if task.event.Event != biz.ActivityEventStreaming {
				// Non-streaming events (created/completed/etc.) must be published
				// immediately so that terminal events are not delayed by the batch window.
				flush()
				s.processTask(activityID, task)
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
			// Sequencer is closing; drain remaining tasks before exiting
			// to ensure events published before Close are not lost.
			for {
				select {
				case task := <-ch:
					if task.event.Event != biz.ActivityEventStreaming {
						flush()
						s.processTask(activityID, task)
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
// coalesced into a single event. Only streaming events for the same activity
// and the same delta_field are merged.
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
// ActivityEvent synchronously.
//
// Phase 1b parallel-async design:
//   - Persist: sent to a single persist worker via a buffered channel
//     (non-blocking). The worker processes items in FIFO order, so per-activity
//     ordering is preserved (start→done). If the channel is full (extreme DB
//     latency), persist falls back to synchronous mode to avoid data loss.
//   - Publish: synchronous. Preserves per-activity FIFO ordering for WS push.
//     WS publish typically completes in <5ms, independent of DB I/O.
//
// On persist failure, the event is still published (fire-and-forget). The
// frontend recovers via API backfill on next reload (eventual consistency).
func (s *activityEventSequencer) processTask(activityID string, task publishTask) {
	if task.persist && s.activityRepo != nil {
		item := persistItem{activityID: activityID, activity: task.activity}
		select {
		case s.persistChan <- item:
			// enqueued for async persist
		default:
			// Channel full (extreme DB latency): fall back to synchronous
			// persist to avoid losing data. This blocks the consumer but
			// only under exceptional conditions.
			s.persistWithRetry(activityID, task.activity, true)
		}
	}
	if s.eventBus != nil {
		s.eventBus.Publish(context.Background(), task.event)
	}
}

// persistWithRetry calls UpsertActivity with exponential backoff retry
// (CS-B15: upper bound + exponential backoff).
//
// Retries up to persistMaxRetries times with exponential backoff starting at
// persistInitialBackoffMs (100ms, 200ms, 400ms, 800ms, 1600ms). On final
// failure, the activity is pushed into the dead-letter buffer so the WS
// reconnect replay path can compensate via ListDeadLetterActivities.
//
// The syncFallback flag is used for log attribution to distinguish between
// the persist worker path and the synchronous fallback path.
func (s *activityEventSequencer) persistWithRetry(activityID string, a biz.Activity, syncFallback bool) {
	backoff := persistInitialBackoffMs
	path := "worker"
	if syncFallback {
		path = "sync_fallback"
	}
	for attempt := 0; attempt <= persistMaxRetries; attempt++ {
		if _, err := s.activityRepo.UpsertActivity(context.Background(), a); err == nil {
			return
		} else if attempt == persistMaxRetries {
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
		// Exponential backoff between attempts. Uses time.Sleep instead of
		// select-on-context because this runs in a background goroutine
		// with no request context to honor.
		time.Sleep(time.Duration(backoff) * time.Millisecond)
		backoff *= persistBackoffFactor
	}
}

// pushDeadLetter appends a failed-persist activity to the dead-letter ring
// buffer. When the buffer is full, the oldest entry is evicted (FIFO eviction).
func (s *activityEventSequencer) pushDeadLetter(a biz.Activity) {
	s.deadLetterMu.Lock()
	defer s.deadLetterMu.Unlock()
	if len(s.deadLetter) >= deadLetterCapacity {
		// Evict oldest (slice header reuse avoids GC pressure).
		s.deadLetter = append(s.deadLetter[:0], s.deadLetter[1:]...)
	}
	s.deadLetter = append(s.deadLetter, a)
}

// ListDeadLetterActivities returns a snapshot of activities whose persist
// failed after all retries, filtered by sessionID. The WS reconnect replay
// path should merge these with ListActivities RPC results to avoid showing
// users a gap for events that were live-delivered but not persisted.
//
// The returned slice is a copy; callers may modify it freely. The buffer
// itself is not cleared (it remains available for diagnostics until the
// process exits or the buffer rolls over).
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

// runPersistWorker is the single persist goroutine that drains persistChan
// sequentially. This guarantees per-activity FIFO ordering: if the consumer
// sends start→done, the worker processes start→done in that exact order.
// The worker exits when persistChan is closed (which happens in Close()
// after all consumers have finished).
//
// Each persist call retries on transient failures via persistWithRetry.
func (s *activityEventSequencer) runPersistWorker() {
	defer s.persistWg.Done()
	for item := range s.persistChan {
		s.persistWithRetry(item.activityID, item.activity, false)
	}
}

// Close closes the sequencer and waits for all consumer goroutines and the
// persist worker to finish.
//
// Shutdown sequence:
//  1. Signal consumers to drain and exit (close s.done)
//  2. Wait for all consumers to finish (s.wg.Wait) — at this point no more
//     items will be sent to persistChan
//  3. Close persistChan — signals the persist worker to exit after draining
//  4. Wait for the persist worker to finish (s.persistWg.Wait)
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

	// Wait for all consumer goroutines to finish draining.
	s.wg.Wait()

	// Now that no consumers remain, close persistChan to signal the
	// persist worker to exit after processing all remaining items.
	if s.persistChan != nil {
		close(s.persistChan)
		s.persistWg.Wait()
	}
}
