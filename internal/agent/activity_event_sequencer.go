package agent

import (
	"context"
	"errors"
	"sync"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event/contract"
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
// activity_delta envelopes for the same field are coalesced into a single
// envelope. This reduces frontend event frequency from "one per token" to
// "at most one per 60fps frame", eliminating UI jank without perceptible lag.
const defaultDeltaBatchInterval = 16 * time.Millisecond

// errSequencerClosed is returned when publishing to a closed sequencer.
var errSequencerClosed = errors.New("activity event sequencer closed")

// activityEventSequencer guarantees per-activity FIFO event ordering.
//
// Each activity gets its own buffered channel and a dedicated consumer
// goroutine. Events for the same activity are strictly ordered
// (start → delta → done), while events for different activities can be
// processed concurrently.
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
	eventBus interface {
		Publish(ctx context.Context, envelope contract.Envelope)
	}
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
}

// persistItem is a single activity to persist, paired with its activity ID
// for logging.
type persistItem struct {
	activityID string
	activity   biz.Activity
}

// publishTask represents a single event to publish and optionally persist.
type publishTask struct {
	env      contract.Envelope
	persist  bool
	activity biz.Activity
}

// newActivityEventSequencer creates a new sequencer.
// The sequencer must be Closed when no longer needed to release goroutines.
func newActivityEventSequencer(
	eventBus interface {
		Publish(ctx context.Context, envelope contract.Envelope)
	},
	lg loggateway.Logger,
) *activityEventSequencer {
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
//  2. Publish the envelope to the event bus
//
// Consecutive activity_delta envelopes for the same field are batched within
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

	mergeDelta := func(dst, src publishTask) {
		dstChunk, _ := dst.env.Metadata["delta_chunk"].(string)
		srcChunk, _ := src.env.Metadata["delta_chunk"].(string)
		dst.env.Metadata["delta_chunk"] = dstChunk + srcChunk
	}

	for {
		select {
		case task, ok := <-ch:
			if !ok {
				return
			}
			if task.env.Type != contract.EnvelopeTypeActivityDelta {
				// Non-delta envelopes (start/done) must be published immediately
				// so that terminal events are not delayed by the batch window.
				flush()
				s.processTask(activityID, task)
				continue
			}
			if pending != nil && s.canMergeDeltas(*pending, task) {
				mergeDelta(*pending, task)
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
					if task.env.Type != contract.EnvelopeTypeActivityDelta {
						flush()
						s.processTask(activityID, task)
						continue
					}
					if pending != nil && s.canMergeDeltas(*pending, task) {
						mergeDelta(*pending, task)
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

// canMergeDeltas reports whether two consecutive publish tasks can be
// coalesced into a single activity_delta envelope. Only delta envelopes for
// the same activity and the same delta_field are merged.
func (s *activityEventSequencer) canMergeDeltas(a, b publishTask) bool {
	if a.env.Type != contract.EnvelopeTypeActivityDelta || b.env.Type != contract.EnvelopeTypeActivityDelta {
		return false
	}
	aField, _ := a.env.Metadata["delta_field"].(string)
	bField, _ := b.env.Metadata["delta_field"].(string)
	if aField == "" || bField == "" || aField != bField {
		return false
	}
	return true
}

// processTask persists the activity (fire-and-forget) and publishes the
// envelope synchronously.
//
// Phase 1b parallel-async design:
//   - Persist: sent to a single persist worker via a buffered channel
//     (non-blocking). The worker processes items in FIFO order, so per-activity
//     ordering is preserved (start→done). If the channel is full (extreme DB
//     latency), persist falls back to synchronous mode to avoid data loss.
//   - Publish: synchronous. Preserves per-activity FIFO ordering for WS push.
//     WS publish typically completes in <5ms, independent of DB I/O.
//
// On persist failure, the envelope is still published (fire-and-forget). The
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
			if _, err := s.activityRepo.UpsertActivity(context.Background(), task.activity); err != nil {
				s.lg.Warn("activity persist failed (sync fallback); frontend will backfill via API",
					loggateway.StepID("agent.activity_sequencer.persist"),
					loggateway.Str("activity_id", activityID),
					loggateway.Str("kind", string(task.activity.Kind)),
					loggateway.Str("status", string(task.activity.Status)),
					loggateway.Err(err))
			}
		}
	}
	if s.eventBus != nil {
		s.eventBus.Publish(context.Background(), task.env)
	}
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
func (s *activityEventSequencer) runPersistWorker() {
	defer s.persistWg.Done()
	for item := range s.persistChan {
		if _, err := s.activityRepo.UpsertActivity(context.Background(), item.activity); err != nil {
			s.lg.Warn("activity persist failed; frontend will backfill via API",
				loggateway.StepID("agent.activity_sequencer.persist"),
				loggateway.Str("activity_id", item.activityID),
				loggateway.Str("kind", string(item.activity.Kind)),
				loggateway.Str("status", string(item.activity.Status)),
				loggateway.Err(err))
		}
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
