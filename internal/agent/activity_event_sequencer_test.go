package agent

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// TestActivityEventSequencer_FIFOOrdering verifies that events for the same
// activity are published in strict FIFO order: created → streaming → completed.
//
// This is the core guarantee that fixes B-01 (start/delta ordering issue).
func TestActivityEventSequencer_FIFOOrdering(t *testing.T) {
	bus := newSyncCaptureBus()
	seq := newActivityEventSequencer(bus, loggateway.NewNoop())
	defer seq.Close()

	activityID := "act-1"
	ctx := context.Background()

	// Publish created → streaming1 → streaming2 → completed for the same activity
	if err := seq.publish(ctx, activityID, publishTask{
		event: biz.ActivityEvent{Event: biz.ActivityEventCreated, Activity: biz.Activity{AgentKey: "agent-1", SessionID: "sess-1"}},
	}); err != nil {
		t.Fatalf("publish created failed: %v", err)
	}
	if err := seq.publish(ctx, activityID, publishTask{
		event: biz.ActivityEvent{Event: biz.ActivityEventStreaming, Activity: biz.Activity{AgentKey: "agent-1", SessionID: "sess-1"}},
	}); err != nil {
		t.Fatalf("publish streaming1 failed: %v", err)
	}
	if err := seq.publish(ctx, activityID, publishTask{
		event: biz.ActivityEvent{Event: biz.ActivityEventStreaming, Activity: biz.Activity{AgentKey: "agent-1", SessionID: "sess-1"}},
	}); err != nil {
		t.Fatalf("publish streaming2 failed: %v", err)
	}
	if err := seq.publish(ctx, activityID, publishTask{
		event: biz.ActivityEvent{Event: biz.ActivityEventCompleted, Activity: biz.Activity{AgentKey: "agent-1", SessionID: "sess-1"}},
	}); err != nil {
		t.Fatalf("publish completed failed: %v", err)
	}

	envs := bus.waitForPublished(t, 4)

	// Verify strict FIFO order
	expected := []biz.ActivityEventType{
		biz.ActivityEventCreated,
		biz.ActivityEventStreaming,
		biz.ActivityEventStreaming,
		biz.ActivityEventCompleted,
	}
	for i, want := range expected {
		if envs[i].Event != want {
			t.Errorf("event[%d] type=%q want %q (FIFO violated)", i, envs[i].Event, want)
		}
	}
}

// TestActivityEventSequencer_ConcurrentActivities verifies that different
// activities can publish concurrently. Events for the same activity maintain
// FIFO order, but events across activities may interleave.
func TestActivityEventSequencer_ConcurrentActivities(t *testing.T) {
	bus := newSyncCaptureBus()
	seq := newActivityEventSequencer(bus, loggateway.NewNoop())
	defer seq.Close()

	ctx := context.Background()
	var wg sync.WaitGroup

	// Activity A: created → streaming → completed
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = seq.publish(ctx, "act-A", publishTask{event: biz.ActivityEvent{Event: biz.ActivityEventCreated, Activity: biz.Activity{AgentKey: "agent-1", SessionID: "sess-1"}}})
		_ = seq.publish(ctx, "act-A", publishTask{event: biz.ActivityEvent{Event: biz.ActivityEventStreaming, Activity: biz.Activity{AgentKey: "agent-1", SessionID: "sess-1"}}})
		_ = seq.publish(ctx, "act-A", publishTask{event: biz.ActivityEvent{Event: biz.ActivityEventCompleted, Activity: biz.Activity{AgentKey: "agent-1", SessionID: "sess-1"}}})
	}()

	// Activity B: created → streaming → completed
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = seq.publish(ctx, "act-B", publishTask{event: biz.ActivityEvent{Event: biz.ActivityEventCreated, Activity: biz.Activity{AgentKey: "agent-2", SessionID: "sess-1"}}})
		_ = seq.publish(ctx, "act-B", publishTask{event: biz.ActivityEvent{Event: biz.ActivityEventStreaming, Activity: biz.Activity{AgentKey: "agent-2", SessionID: "sess-1"}}})
		_ = seq.publish(ctx, "act-B", publishTask{event: biz.ActivityEvent{Event: biz.ActivityEventCompleted, Activity: biz.Activity{AgentKey: "agent-2", SessionID: "sess-1"}}})
	}()

	wg.Wait()
	envs := bus.waitForPublished(t, 6)

	// Extract per-activity event sequences
	var actA, actB []biz.ActivityEventType
	for _, ev := range envs {
		// Use AgentKey to distinguish activities (act-A uses agent-1, act-B uses agent-2)
		if ev.Activity.AgentKey == "agent-1" {
			actA = append(actA, ev.Event)
		} else if ev.Activity.AgentKey == "agent-2" {
			actB = append(actB, ev.Event)
		}
	}

	// Verify each activity's FIFO order
	expectedSeq := []biz.ActivityEventType{
		biz.ActivityEventCreated,
		biz.ActivityEventStreaming,
		biz.ActivityEventCompleted,
	}
	if len(actA) != 3 {
		t.Fatalf("activity A: expected 3 events, got %d", len(actA))
	}
	for i, want := range expectedSeq {
		if actA[i] != want {
			t.Errorf("activity A event[%d]=%q want %q (FIFO violated)", i, actA[i], want)
		}
	}
	if len(actB) != 3 {
		t.Fatalf("activity B: expected 3 events, got %d", len(actB))
	}
	for i, want := range expectedSeq {
		if actB[i] != want {
			t.Errorf("activity B event[%d]=%q want %q (FIFO violated)", i, actB[i], want)
		}
	}
}

// TestActivityEventSequencer_Persistence verifies that when persist=true,
// the activity is persisted via the ActivityWriter.
func TestActivityEventSequencer_Persistence(t *testing.T) {
	bus := newSyncCaptureBus()
	repo := newMockActivityWriter()
	seq := newActivityEventSequencer(bus, loggateway.NewNoop())
	seq.SetActivityRepo(repo)
	defer seq.Close()

	ctx := context.Background()
	activity := biz.Activity{
		ID:      "act-persist-1",
		Kind:    biz.ActivityKindReply,
		Status:  biz.ActivityStatusCompleted,
		Content: "Hello world",
	}

	if err := seq.publish(ctx, activity.ID, publishTask{
		event:    biz.ActivityEvent{Event: biz.ActivityEventCompleted, Activity: activity},
		persist:  true,
		activity: activity,
	}); err != nil {
		t.Fatalf("publish failed: %v", err)
	}

	// Wait for publish
	bus.waitForPublished(t, 1)

	// Wait for persistence (may be async)
	deadline := time.After(2 * time.Second)
	for {
		repo.mu.Lock()
		_, ok := repo.activities[activity.ID]
		repo.mu.Unlock()
		if ok {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for activity persistence")
		case <-time.After(10 * time.Millisecond):
		}
	}

	repo.mu.Lock()
	defer repo.mu.Unlock()
	got, ok := repo.activities[activity.ID]
	if !ok {
		t.Fatalf("activity not persisted")
	}
	if got.Content != activity.Content {
		t.Errorf("persisted content=%q want %q", got.Content, activity.Content)
	}
	if got.Kind != activity.Kind {
		t.Errorf("persisted kind=%q want %q", got.Kind, activity.Kind)
	}
}

// TestActivityEventSequencer_CloseWaitsForGoroutines verifies that Close
// blocks until all consumer goroutines have finished processing queued tasks.
func TestActivityEventSequencer_CloseWaitsForGoroutines(t *testing.T) {
	bus := newSyncCaptureBus()
	seq := newActivityEventSequencer(bus, loggateway.NewNoop())

	ctx := context.Background()
	// Publish events for multiple activities
	for i := 0; i < 5; i++ {
		activityID := "act-" + string(rune('A'+i))
		_ = seq.publish(ctx, activityID, publishTask{
			event: biz.ActivityEvent{Event: biz.ActivityEventCreated, Activity: biz.Activity{AgentKey: "agent-1", SessionID: "sess-1"}},
		})
		_ = seq.publish(ctx, activityID, publishTask{
			event: biz.ActivityEvent{Event: biz.ActivityEventCompleted, Activity: biz.Activity{AgentKey: "agent-1", SessionID: "sess-1"}},
		})
	}

	// Close should wait for all 10 events to be published
	seq.Close()

	// After Close, all events should be published
	bus.mu.Lock()
	count := len(bus.published)
	bus.mu.Unlock()
	if count != 10 {
		t.Errorf("after Close: published count=%d want 10", count)
	}
}

// TestActivityEventSequencer_PublishAfterCloseReturnsError verifies that
// publishing after Close returns an error.
func TestActivityEventSequencer_PublishAfterCloseReturnsError(t *testing.T) {
	bus := newSyncCaptureBus()
	seq := newActivityEventSequencer(bus, loggateway.NewNoop())
	seq.Close()

	err := seq.publish(context.Background(), "act-1", publishTask{
		event: biz.ActivityEvent{Event: biz.ActivityEventCreated, Activity: biz.Activity{AgentKey: "agent-1", SessionID: "sess-1"}},
	})
	if err == nil {
		t.Errorf("expected error when publishing after Close, got nil")
	}
}

// TestActivityEventSequencer_Backpressure verifies that when the channel is
// full, publish blocks (backpressure). This is the mechanism that propagates
// backpressure to the LLM: channel full → OnTextDelta blocks → stream_consumer
// blocks → LLM pauses.
func TestActivityEventSequencer_Backpressure(t *testing.T) {
	// Create a sequencer with a bus that blocks publishing.
	// publishCalled signals when the consumer has entered Publish (blocked).
	// publishCh unblocks the consumer when closed.
	bus := &blockingBus{
		publishCalled: make(chan struct{}, 1),
		publishCh:     make(chan struct{}),
	}
	seq := newActivityEventSequencer(bus, loggateway.NewNoop())
	defer seq.Close()

	activityID := "act-backpressure"

	// First publish creates the channel and starts the consumer goroutine.
	// The consumer reads this task and blocks on bus.Publish.
	_ = seq.publish(context.Background(), activityID, publishTask{
		event: biz.ActivityEvent{Event: biz.ActivityEventStreaming, Activity: biz.Activity{AgentKey: "agent-1", SessionID: "sess-1"}},
	})

	// Wait until the consumer has called bus.Publish (and is now blocked).
	<-bus.publishCalled

	// Consumer is blocked. Fill the queue buffer (defaultPublishBufferSize tasks).
	for i := 0; i < defaultPublishBufferSize; i++ {
		_ = seq.publish(context.Background(), activityID, publishTask{
			event: biz.ActivityEvent{Event: biz.ActivityEventStreaming, Activity: biz.Activity{AgentKey: "agent-1", SessionID: "sess-1"}},
		})
	}

	// Buffer is now full. This publish should block until ctx timeout.
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := seq.publish(ctx, activityID, publishTask{
		event: biz.ActivityEvent{Event: biz.ActivityEventCompleted, Activity: biz.Activity{AgentKey: "agent-1", SessionID: "sess-1"}},
	})

	// Should return context.DeadlineExceeded (blocked until timeout)
	if err == nil {
		t.Errorf("expected timeout error due to backpressure, got nil")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected context.DeadlineExceeded, got %v", err)
	}

	// Unblock the consumer so Close can proceed
	close(bus.publishCh)
}

// TestActivityEventSequencer_ContextCancellation verifies that publish
// returns ctx.Err() when the context is cancelled.
func TestActivityEventSequencer_ContextCancellation(t *testing.T) {
	bus := &blockingBus{
		publishCalled: make(chan struct{}, 1),
		publishCh:     make(chan struct{}),
	}
	seq := newActivityEventSequencer(bus, loggateway.NewNoop())
	defer seq.Close()

	ctx, cancel := context.WithCancel(context.Background())
	activityID := "act-cancel"

	// First publish creates channel and starts consumer (which blocks on bus.Publish)
	_ = seq.publish(context.Background(), activityID, publishTask{
		event: biz.ActivityEvent{Event: biz.ActivityEventStreaming, Activity: biz.Activity{AgentKey: "agent-1", SessionID: "sess-1"}},
	})

	// Wait until consumer is blocked on bus.Publish
	<-bus.publishCalled

	// Fill the publish queue buffer
	for i := 0; i < defaultPublishBufferSize; i++ {
		_ = seq.publish(context.Background(), activityID, publishTask{
			event: biz.ActivityEvent{Event: biz.ActivityEventStreaming, Activity: biz.Activity{AgentKey: "agent-1", SessionID: "sess-1"}},
		})
	}

	// Cancel context before publishing
	cancel()
	err := seq.publish(ctx, activityID, publishTask{
		event: biz.ActivityEvent{Event: biz.ActivityEventCompleted, Activity: biz.Activity{AgentKey: "agent-1", SessionID: "sess-1"}},
	})

	if err == nil {
		t.Errorf("expected error when context cancelled, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}

	// Unblock the consumer so Close can proceed
	close(bus.publishCh)
}

// blockingBus is a biz.ActivityEventBus that blocks on Publish until publishCh is closed.
// publishCalled signals when the consumer has entered Publish (and is blocked).
// Used to simulate backpressure for testing.
type blockingBus struct {
	publishCalled chan struct{}
	publishCh     chan struct{}
}

func (b *blockingBus) Publish(_ context.Context, _ biz.ActivityEvent) {
	select {
	case b.publishCalled <- struct{}{}:
	default:
	}
	<-b.publishCh
}

func (b *blockingBus) Subscribe(_ biz.ActivityEventSubscribeOptions) (<-chan biz.ActivityEvent, func()) {
	return nil, func() {}
}

func (b *blockingBus) DropCount() uint64 { return 0 }

// deltaTask creates a publishTask for a streaming event.
func deltaTask(field, chunk string) publishTask {
	return publishTask{
		event: biz.ActivityEvent{
			Event:      biz.ActivityEventStreaming,
			DeltaField: field,
			DeltaChunk: chunk,
		},
	}
}

// TestActivityEventSequencer_DeltaBatching verifies that consecutive
// streaming events for the same field are coalesced into a single
// event, reducing frontend event frequency.
func TestActivityEventSequencer_DeltaBatching(t *testing.T) {
	bus := newSyncCaptureBus()
	seq := newActivityEventSequencer(bus, loggateway.NewNoop())
	seq.deltaBatchInterval = 5 * time.Millisecond
	defer seq.Close()

	activityID := "act-batch"
	ctx := context.Background()

	_ = seq.publish(ctx, activityID, publishTask{
		event: biz.ActivityEvent{Event: biz.ActivityEventCreated, Activity: biz.Activity{AgentKey: "agent-1", SessionID: "sess-1"}},
	})
	_ = seq.publish(ctx, activityID, deltaTask("content", "a"))
	_ = seq.publish(ctx, activityID, deltaTask("content", "b"))
	_ = seq.publish(ctx, activityID, deltaTask("content", "c"))
	_ = seq.publish(ctx, activityID, publishTask{
		event: biz.ActivityEvent{Event: biz.ActivityEventCompleted, Activity: biz.Activity{AgentKey: "agent-1", SessionID: "sess-1"}},
	})

	envs := bus.waitForPublished(t, 3)

	expected := []biz.ActivityEventType{
		biz.ActivityEventCreated,
		biz.ActivityEventStreaming,
		biz.ActivityEventCompleted,
	}
	for i, want := range expected {
		if envs[i].Event != want {
			t.Errorf("event[%d] type=%q want %q", i, envs[i].Event, want)
		}
	}
	if envs[1].DeltaChunk != "abc" {
		t.Errorf("batched delta chunk=%q want %q", envs[1].DeltaChunk, "abc")
	}
}

// TestActivityEventSequencer_DeltaBatchingDifferentFields verifies that delta
// events for different fields are not merged.
func TestActivityEventSequencer_DeltaBatchingDifferentFields(t *testing.T) {
	bus := newSyncCaptureBus()
	seq := newActivityEventSequencer(bus, loggateway.NewNoop())
	seq.deltaBatchInterval = 5 * time.Millisecond
	defer seq.Close()

	activityID := "act-fields"
	ctx := context.Background()

	_ = seq.publish(ctx, activityID, deltaTask("content", "a"))
	_ = seq.publish(ctx, activityID, deltaTask("reasoning", "b"))
	_ = seq.publish(ctx, activityID, deltaTask("content", "c"))

	envs := bus.waitForPublished(t, 3)

	chunks := []string{
		envs[0].DeltaChunk,
		envs[1].DeltaChunk,
		envs[2].DeltaChunk,
	}
	want := []string{"a", "b", "c"}
	for i, w := range want {
		if chunks[i] != w {
			t.Errorf("event[%d] chunk=%q want %q", i, chunks[i], w)
		}
	}
}

// TestActivityEventSequencer_DeltaBatchingTimerFlush verifies that a single
// streaming event is flushed after the batch interval expires.
func TestActivityEventSequencer_DeltaBatchingTimerFlush(t *testing.T) {
	bus := newSyncCaptureBus()
	seq := newActivityEventSequencer(bus, loggateway.NewNoop())
	seq.deltaBatchInterval = 10 * time.Millisecond
	defer seq.Close()

	activityID := "act-timer"
	ctx := context.Background()

	_ = seq.publish(ctx, activityID, deltaTask("content", "x"))

	envs := bus.waitForPublished(t, 1)
	if len(envs) != 1 {
		t.Fatalf("expected 1 event, got %d", len(envs))
	}
	if envs[0].DeltaChunk != "x" {
		t.Errorf("chunk=%q want %q", envs[0].DeltaChunk, "x")
	}
}

// TestActivityEventSequencer_DeltaBatchingFlushOnNonDelta verifies that the
// pending batched delta is flushed immediately when a non-delta event arrives.
func TestActivityEventSequencer_DeltaBatchingFlushOnNonDelta(t *testing.T) {
	bus := newSyncCaptureBus()
	seq := newActivityEventSequencer(bus, loggateway.NewNoop())
	seq.deltaBatchInterval = 100 * time.Millisecond
	defer seq.Close()

	activityID := "act-flush"
	ctx := context.Background()

	_ = seq.publish(ctx, activityID, deltaTask("content", "a"))
	_ = seq.publish(ctx, activityID, deltaTask("content", "b"))
	_ = seq.publish(ctx, activityID, publishTask{
		event: biz.ActivityEvent{Event: biz.ActivityEventCompleted, Activity: biz.Activity{AgentKey: "agent-1", SessionID: "sess-1"}},
	})

	envs := bus.waitForPublished(t, 2)
	if envs[0].Event != biz.ActivityEventStreaming {
		t.Errorf("first event type=%q want streaming", envs[0].Event)
	}
	if envs[0].DeltaChunk != "ab" {
		t.Errorf("batched chunk=%q want %q", envs[0].DeltaChunk, "ab")
	}
	if envs[1].Event != biz.ActivityEventCompleted {
		t.Errorf("second event type=%q want completed", envs[1].Event)
	}
}

// --- D2: persist retry + dead-letter compensation tests ---

// failingActivityWriter is a biz.ActivityWriter whose UpsertActivity always
// returns an error. Used to exercise the retry → dead-letter path.
type failingActivityWriter struct {
	attempts int32 // atomic; total UpsertActivity calls
}

func (m *failingActivityWriter) CreateActivity(_ context.Context, a biz.Activity) (biz.Activity, error) {
	atomic.AddInt32(&m.attempts, 1)
	return biz.Activity{}, errors.New("simulated DB failure")
}
func (m *failingActivityWriter) UpdateActivity(_ context.Context, a biz.Activity) (biz.Activity, error) {
	atomic.AddInt32(&m.attempts, 1)
	return biz.Activity{}, errors.New("simulated DB failure")
}
func (m *failingActivityWriter) UpsertActivity(_ context.Context, a biz.Activity) (biz.Activity, error) {
	atomic.AddInt32(&m.attempts, 1)
	return biz.Activity{}, errors.New("simulated DB failure")
}

// flakyActivityWriter fails the first failUntil UpsertActivity attempts, then
// succeeds. Used to verify retry-then-succeed behavior.
type flakyActivityWriter struct {
	mu         sync.Mutex
	failUntil  int
	attempts   int
	activities map[string]biz.Activity
}

func newFlakyActivityWriter(failUntil int) *flakyActivityWriter {
	return &flakyActivityWriter{failUntil: failUntil, activities: make(map[string]biz.Activity)}
}

func (m *flakyActivityWriter) CreateActivity(ctx context.Context, a biz.Activity) (biz.Activity, error) {
	return m.UpsertActivity(ctx, a)
}
func (m *flakyActivityWriter) UpdateActivity(ctx context.Context, a biz.Activity) (biz.Activity, error) {
	return m.UpsertActivity(ctx, a)
}
func (m *flakyActivityWriter) UpsertActivity(_ context.Context, a biz.Activity) (biz.Activity, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.attempts++
	if m.attempts <= m.failUntil {
		return biz.Activity{}, errors.New("simulated transient failure")
	}
	m.activities[a.ID] = a
	return a, nil
}

// fastRetryConfig overrides the sequencer's retry parameters for fast test
// execution (1ms backoff instead of 100ms).
func fastRetryConfig(seq *activityEventSequencer) {
	seq.persistMaxRetries = 2
	seq.persistInitialBackoffMs = 1
	seq.persistBackoffFactor = 2
}

// TestPersistWithRetry_ExhaustionToDeadLetter verifies that when all retries
// are exhausted, the activity lands in the dead-letter buffer.
func TestPersistWithRetry_ExhaustionToDeadLetter(t *testing.T) {
	bus := newSyncCaptureBus()
	repo := &failingActivityWriter{}
	seq := newActivityEventSequencer(bus, loggateway.NewNoop())
	fastRetryConfig(seq)
	seq.SetActivityRepo(repo)

	activity := biz.Activity{ID: "act-dl-1", Kind: biz.ActivityKindReply, Status: biz.ActivityStatusCompleted, SessionID: "sess-1"}
	if err := seq.publish(context.Background(), activity.ID, publishTask{
		event:    biz.ActivityEvent{Event: biz.ActivityEventCompleted, Activity: activity},
		persist:  true,
		activity: activity,
	}); err != nil {
		t.Fatalf("publish failed: %v", err)
	}
	bus.waitForPublished(t, 1)

	// Wait for dead-letter (retries are fast: 1ms+2ms = 3ms total backoff).
	deadline := time.After(2 * time.Second)
	for {
		if len(seq.ListDeadLetterActivities("sess-1")) > 0 {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for dead-letter entry")
		case <-time.After(5 * time.Millisecond):
		}
	}

	dl := seq.ListDeadLetterActivities("sess-1")
	if len(dl) != 1 {
		t.Fatalf("dead-letter count=%d want 1", len(dl))
	}
	if dl[0].ID != activity.ID {
		t.Errorf("dead-letter activity ID=%q want %q", dl[0].ID, activity.ID)
	}
	// maxRetries=2 means 3 total attempts (0,1,2).
	if got := atomic.LoadInt32(&repo.attempts); got != 3 {
		t.Errorf("UpsertActivity attempts=%d want 3", got)
	}
	seq.Close()
}

// TestPersistWithRetry_RetryThenSucceed verifies that transient failures are
// retried and the activity is persisted once the repo recovers.
func TestPersistWithRetry_RetryThenSucceed(t *testing.T) {
	bus := newSyncCaptureBus()
	repo := newFlakyActivityWriter(2) // fail first 2 attempts, succeed on 3rd
	seq := newActivityEventSequencer(bus, loggateway.NewNoop())
	fastRetryConfig(seq)
	seq.SetActivityRepo(repo)

	activity := biz.Activity{ID: "act-retry-ok", Kind: biz.ActivityKindReply, SessionID: "sess-1"}
	if err := seq.publish(context.Background(), activity.ID, publishTask{
		event:    biz.ActivityEvent{Event: biz.ActivityEventCompleted, Activity: activity},
		persist:  true,
		activity: activity,
	}); err != nil {
		t.Fatalf("publish failed: %v", err)
	}
	bus.waitForPublished(t, 1)

	// Wait for persistence to complete.
	deadline := time.After(2 * time.Second)
	for {
		repo.mu.Lock()
		_, ok := repo.activities[activity.ID]
		repo.mu.Unlock()
		if ok {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for activity persistence")
		case <-time.After(5 * time.Millisecond):
		}
	}

	// Activity must NOT be in dead-letter (persist succeeded).
	if dl := seq.ListDeadLetterActivities("sess-1"); len(dl) != 0 {
		t.Errorf("dead-letter count=%d want 0 (persist should have succeeded)", len(dl))
	}
	seq.Close()
}

// TestPushDeadLetter_Dedup verifies that pushing the same activity ID twice
// keeps only the latest snapshot (S4 fix).
func TestPushDeadLetter_Dedup(t *testing.T) {
	seq := newActivityEventSequencer(newSyncCaptureBus(), loggateway.NewNoop())
	defer seq.Close()

	// Push an initial "running" snapshot.
	seq.pushDeadLetter(biz.Activity{ID: "act-dedup", Status: biz.ActivityStatusRunning, Content: "old"})

	// Push a newer "failed" snapshot for the same ID.
	seq.pushDeadLetter(biz.Activity{ID: "act-dedup", Status: biz.ActivityStatusFailed, Content: "new"})

	dl := seq.ListDeadLetterActivities("")
	if len(dl) != 1 {
		t.Fatalf("dead-letter count=%d want 1 (dedup by activity ID)", len(dl))
	}
	if dl[0].Status != biz.ActivityStatusFailed {
		t.Errorf("dead-letter status=%q want %q (latest snapshot)", dl[0].Status, biz.ActivityStatusFailed)
	}
	if dl[0].Content != "new" {
		t.Errorf("dead-letter content=%q want %q (latest snapshot)", dl[0].Content, "new")
	}
}

// TestPushDeadLetter_Eviction verifies that when the ring buffer is full, the
// oldest entry is evicted (FIFO).
func TestPushDeadLetter_Eviction(t *testing.T) {
	seq := newActivityEventSequencer(newSyncCaptureBus(), loggateway.NewNoop())
	defer seq.Close()

	// Fill the buffer to capacity with unique IDs.
	for i := 0; i < deadLetterCapacity; i++ {
		seq.pushDeadLetter(biz.Activity{ID: "act-evict-" + strconv.Itoa(i), SessionID: "sess-1"})
	}
	// Push one more — should evict the oldest (act-evict-0).
	seq.pushDeadLetter(biz.Activity{ID: "act-evict-new", SessionID: "sess-1"})

	dl := seq.ListDeadLetterActivities("sess-1")
	if len(dl) != deadLetterCapacity {
		t.Fatalf("dead-letter count=%d want %d (capacity)", len(dl), deadLetterCapacity)
	}
	// The oldest entry (act-evict-0) must have been evicted.
	for _, a := range dl {
		if a.ID == "act-evict-0" {
			t.Errorf("act-evict-0 should have been evicted (FIFO)")
		}
	}
	// The new entry must be present.
	found := false
	for _, a := range dl {
		if a.ID == "act-evict-new" {
			found = true
		}
	}
	if !found {
		t.Errorf("act-evict-new not found in dead-letter buffer")
	}
}

// TestListDeadLetterActivities_SessionFilter verifies that the sessionID
// filter correctly scopes the returned snapshot.
func TestListDeadLetterActivities_SessionFilter(t *testing.T) {
	seq := newActivityEventSequencer(newSyncCaptureBus(), loggateway.NewNoop())
	defer seq.Close()

	seq.pushDeadLetter(biz.Activity{ID: "act-a", SessionID: "sess-1"})
	seq.pushDeadLetter(biz.Activity{ID: "act-b", SessionID: "sess-2"})
	seq.pushDeadLetter(biz.Activity{ID: "act-c", SessionID: "sess-1"})

	// Filter by sess-1.
	dl := seq.ListDeadLetterActivities("sess-1")
	if len(dl) != 2 {
		t.Fatalf("sess-1 dead-letter count=%d want 2", len(dl))
	}
	for _, a := range dl {
		if a.SessionID != "sess-1" {
			t.Errorf("dead-letter sessionID=%q want sess-1", a.SessionID)
		}
	}

	// Empty sessionID returns all.
	if got := seq.ListDeadLetterActivities(""); len(got) != 3 {
		t.Errorf("empty-filter dead-letter count=%d want 3", len(got))
	}
}

// TestPersistWithRetry_CloseInterruptsBackoff verifies that Close() does not
// block on retry backoff when the DB is unavailable (S3 fix). Without the
// fix, Close would wait for the full backoff budget (500ms+ per item).
func TestPersistWithRetry_CloseInterruptsBackoff(t *testing.T) {
	bus := newSyncCaptureBus()
	repo := &failingActivityWriter{}
	seq := newActivityEventSequencer(bus, loggateway.NewNoop())
	seq.SetActivityRepo(repo)
	// Long backoff so that without the fix, Close would block for 500ms+.
	seq.persistMaxRetries = 5
	seq.persistInitialBackoffMs = 500
	seq.persistBackoffFactor = 2

	activity := biz.Activity{ID: "act-close", Kind: biz.ActivityKindReply, SessionID: "sess-1"}
	if err := seq.publish(context.Background(), activity.ID, publishTask{
		event:    biz.ActivityEvent{Event: biz.ActivityEventCompleted, Activity: activity},
		persist:  true,
		activity: activity,
	}); err != nil {
		t.Fatalf("publish failed: %v", err)
	}
	bus.waitForPublished(t, 1)
	// Give the persist worker time to fail its first attempt and enter the
	// 500ms backoff sleep.
	time.Sleep(50 * time.Millisecond)

	// Close must complete quickly — the backoff must be interrupted by done.
	start := time.Now()
	seq.Close()
	elapsed := time.Since(start)

	// Without the fix, Close blocks for ≥500ms (first backoff). With the fix,
	// done is closed and the backoff is aborted immediately.
	if elapsed > 200*time.Millisecond {
		t.Errorf("Close took %v; expected <200ms (backoff should be interrupted by done)", elapsed)
	}

	// The activity should be in dead-letter (aborted during shutdown).
	if dl := seq.ListDeadLetterActivities("sess-1"); len(dl) != 1 {
		t.Errorf("dead-letter count=%d want 1 (aborted persist should land in dead-letter)", len(dl))
	}
}

// TestProcessTask_SyncFallback verifies that when persistChan is full, the
// consumer falls back to synchronous persist (processTask → persistWithRetry
// with syncFallback=true) rather than dropping the item.
func TestProcessTask_SyncFallback(t *testing.T) {
	bus := newSyncCaptureBus()
	repo := &failingActivityWriter{}
	seq := newActivityEventSequencer(bus, loggateway.NewNoop())
	fastRetryConfig(seq)
	// Manually wire up a full persistChan without starting the worker, so
	// processTask's non-blocking send hits the default (sync fallback) path.
	seq.activityRepo = repo
	seq.persistChan = make(chan persistItem, 1)
	seq.persistChan <- persistItem{activityID: "blocker", activity: biz.Activity{ID: "blocker"}}

	// processTask should detect the full channel and fall back to sync persist.
	activity := biz.Activity{ID: "act-sync", Kind: biz.ActivityKindReply, SessionID: "sess-1"}
	seq.processTask(publishTask{
		event:    biz.ActivityEvent{Event: biz.ActivityEventCompleted, Activity: activity},
		persist:  true,
		activity: activity,
	})

	// The sync fallback called persistWithRetry(syncFallback=true), which
	// exhausted retries and pushed to dead-letter.
	dl := seq.ListDeadLetterActivities("sess-1")
	found := false
	for _, a := range dl {
		if a.ID == activity.ID {
			found = true
		}
	}
	if !found {
		t.Errorf("act-sync not found in dead-letter; sync fallback may not have been triggered")
	}
	// The event must still have been published (fire-and-forget).
	if got := len(bus.published); got != 1 {
		t.Errorf("published count=%d want 1 (publish must still happen on sync fallback)", got)
	}
	seq.Close()
}
