package agent

import (
	"context"
	"errors"
	"sync"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event/contract"
	"aranea-agents/pkg/loggateway"
)

// defaultChannelBufferSize is the per-activity channel buffer size.
// When full, publish blocks (backpressure) which propagates to the LLM:
// channel full → OnTextDelta blocks → stream_consumer blocks → LLM pauses.
const defaultChannelBufferSize = 64

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
	mu           sync.Mutex
	channels     map[string]chan publishTask
	eventBus     interface {
		Publish(ctx context.Context, envelope contract.Envelope)
	}
	activityRepo biz.ActivityWriter
	lg           loggateway.Logger
	wg           sync.WaitGroup
	closed       bool
	done         chan struct{}
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
		channels: make(map[string]chan publishTask),
		eventBus: eventBus,
		lg:       lg,
		done:     make(chan struct{}),
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
		go s.consume(activityID, ch)
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
//  1. Publish the envelope to the event bus
//  2. Persist the activity to the database (if persist=true)
//
// The goroutine exits when the channel is drained and closed, or when the
// done signal is received (on Close).
func (s *activityEventSequencer) consume(activityID string, ch <-chan publishTask) {
	defer s.wg.Done()
	for {
		select {
		case task, ok := <-ch:
			if !ok {
				return
			}
			s.processTask(activityID, task)
		case <-s.done:
			// Sequencer is closing; drain remaining tasks before exiting
			// to ensure events published before Close are not lost.
			for {
				select {
				case task := <-ch:
					s.processTask(activityID, task)
				default:
					return
				}
			}
		}
	}
}

// processTask publishes the envelope and optionally persists the activity.
func (s *activityEventSequencer) processTask(activityID string, task publishTask) {
	if s.eventBus != nil {
		s.eventBus.Publish(context.Background(), task.env)
	}
	if task.persist && s.activityRepo != nil {
		if _, err := s.activityRepo.UpsertActivity(context.Background(), task.activity); err != nil {
			s.lg.Warn("activity persist failed",
				loggateway.StepID("agent.activity_sequencer.persist"),
				loggateway.Str("activity_id", activityID),
				loggateway.Str("kind", string(task.activity.Kind)),
				loggateway.Str("status", string(task.activity.Status)),
				loggateway.Err(err))
		}
	}
}

// Close closes the sequencer and waits for all consumer goroutines to finish.
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
	s.wg.Wait()
}
