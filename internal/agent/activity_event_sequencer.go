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
	activityRepo biz.ActivityUpserter
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

	// activityVersion tracks the next monotonic version for each activity ID.
	// Updated and read only by the single publish worker, guarded by s.mu for
	// races against Close()/publish() visibility.
	activityVersion map[string]int64
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
		activityVersion:         make(map[string]int64),
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

	// Do NOT select on ctx.Done(): activity publish+persist is an async
	// operation that must complete even when the caller's context is canceled
	// (e.g. turnCtx canceled during stream draining). Previously, when
	// turnCtx was canceled, Go's select randomly chose between the publishQueue
	// send and ctx.Done(), dropping ~50% of action activities (ToolResult/
	// completed events) during the drain phase — causing tools to disappear
	// after page refresh because they were never persisted to the DB.
	//
	// The publishQueue has a 256-item buffer; if it ever fills up, blocking
	// here applies natural backpressure rather than silently dropping events.
	select {
	case s.publishQueue <- task:
		return nil
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
//
// 关键约束（P-02 修复）：必须检查 Activity.ID 相同。16ms 批窗口内可能到达
// 来自不同 Activity 的同字段流式事件（如多 member 并发 reply，DeltaField 都是
// "content"）。若不检查 ID，mergeDelta 会把 member2 的 DeltaChunk 追加到
// member1 的事件中，而 member1 的 Activity 快照保持不变 → 前端把 member2 的
// 文本渲染到 member1 的 ReplyBlock，member2 的 Activity 永远收不到自己的 chunk。
func (s *activityEventSequencer) canMergeDeltas(a, b publishTask) bool {
	if a.event.Event != biz.ActivityEventStreaming || b.event.Event != biz.ActivityEventStreaming {
		return false
	}
	if a.event.DeltaField == "" || b.event.DeltaField == "" || a.event.DeltaField != b.event.DeltaField {
		return false
	}
	// P-02: 只有同一 Activity 的流式分片才能合并。跨 Activity 合并会导致
	// 文本串台（member2 的内容被追加到 member1 的事件中）。
	return a.event.Activity.ID == b.event.Activity.ID
}

// processTask persists the activity (fire-and-forget) and publishes the
// ActivityEvent synchronously. Called from the single publish worker —
// serializes all eventBus.Publish calls.
//
// Before publishing, the event is marked SequencerHandled=true so the
// ActivityEventBus knows it has already been persisted by this sequencer
// (via the persist worker above) and skips its own direct-publish handling
// (SessionID normalization + async UpsertActivity). This prevents
// double-persist and avoids the bus overwriting the original (non-redacted)
// activity data with the redacted snapshot that the bus receives.
//
// A monotonic version is assigned to each activity update so that the async
// persist worker can use version-guarded upserts: stale updates (lower version)
// are rejected by the repository, ensuring the DB reflects the same order as
// the publish worker. The version is stamped on both the persisted copy and
// the published event so their ordering is consistent.
func (s *activityEventSequencer) processTask(task publishTask) {
	s.mu.Lock()
	s.activityVersion[task.activity.ID]++
	version := s.activityVersion[task.activity.ID]
	s.mu.Unlock()
	task.activity.Version = version
	task.event.Activity.Version = version

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
		task.event.SequencerHandled = true
		s.eventBus.Publish(context.Background(), task.event)
	}
}

// persistWithRetry calls UpsertActivity with exponential backoff retry.
//
// Note: This function deliberately does NOT abort on s.done (Close). The
// persist worker is drained by Close() via persistWg.Wait(), which blocks
// until all queued items are persisted. Aborting on s.done caused action
// activities (which have multiple upserts: created → streaming delta →
// completed) to be pushed to the dead-letter buffer instead of the DB when
// Close() was called during finalize() — resulting in tools disappearing
// after page refresh (API loads from DB, WS delivers live).
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
		time.Sleep(time.Duration(backoff) * time.Millisecond)
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
func (s *activityEventSequencer) SetActivityRepo(repo biz.ActivityUpserter) {
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
