package agent

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event/contract"
	"aranea-agents/pkg/loggateway"
)

// TestActivityEventSequencer_FIFOOrdering verifies that events for the same
// activity are published in strict FIFO order: start → delta → done.
//
// This is the core guarantee that fixes B-01 (start/delta ordering issue).
func TestActivityEventSequencer_FIFOOrdering(t *testing.T) {
	bus := newSyncCaptureBus()
	seq := newActivityEventSequencer(bus, loggateway.NewNoop())
	defer seq.Close()

	activityID := "act-1"
	ctx := context.Background()

	// Publish start → delta1 → delta2 → done for the same activity
	if err := seq.publish(ctx, activityID, publishTask{
		env: contract.NewEnvelope(contract.EnvelopeTypeActivityStart, "agent-1", "sess-1"),
	}); err != nil {
		t.Fatalf("publish start failed: %v", err)
	}
	if err := seq.publish(ctx, activityID, publishTask{
		env: contract.NewEnvelope(contract.EnvelopeTypeActivityDelta, "agent-1", "sess-1"),
	}); err != nil {
		t.Fatalf("publish delta1 failed: %v", err)
	}
	if err := seq.publish(ctx, activityID, publishTask{
		env: contract.NewEnvelope(contract.EnvelopeTypeActivityDelta, "agent-1", "sess-1"),
	}); err != nil {
		t.Fatalf("publish delta2 failed: %v", err)
	}
	if err := seq.publish(ctx, activityID, publishTask{
		env: contract.NewEnvelope(contract.EnvelopeTypeActivityDone, "agent-1", "sess-1"),
	}); err != nil {
		t.Fatalf("publish done failed: %v", err)
	}

	envs := bus.waitForPublished(t, 4)

	// Verify strict FIFO order
	expected := []contract.EnvelopeType{
		contract.EnvelopeTypeActivityStart,
		contract.EnvelopeTypeActivityDelta,
		contract.EnvelopeTypeActivityDelta,
		contract.EnvelopeTypeActivityDone,
	}
	for i, want := range expected {
		if envs[i].Type != want {
			t.Errorf("envelope[%d] type=%q want %q (FIFO violated)", i, envs[i].Type, want)
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

	// Activity A: start → delta → done
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = seq.publish(ctx, "act-A", publishTask{env: contract.NewEnvelope(contract.EnvelopeTypeActivityStart, "agent-1", "sess-1")})
		_ = seq.publish(ctx, "act-A", publishTask{env: contract.NewEnvelope(contract.EnvelopeTypeActivityDelta, "agent-1", "sess-1")})
		_ = seq.publish(ctx, "act-A", publishTask{env: contract.NewEnvelope(contract.EnvelopeTypeActivityDone, "agent-1", "sess-1")})
	}()

	// Activity B: start → delta → done
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = seq.publish(ctx, "act-B", publishTask{env: contract.NewEnvelope(contract.EnvelopeTypeActivityStart, "agent-2", "sess-1")})
		_ = seq.publish(ctx, "act-B", publishTask{env: contract.NewEnvelope(contract.EnvelopeTypeActivityDelta, "agent-2", "sess-1")})
		_ = seq.publish(ctx, "act-B", publishTask{env: contract.NewEnvelope(contract.EnvelopeTypeActivityDone, "agent-2", "sess-1")})
	}()

	wg.Wait()
	envs := bus.waitForPublished(t, 6)

	// Extract per-activity event sequences
	var actA, actB []contract.EnvelopeType
	for _, env := range envs {
		// Use Author to distinguish activities (act-A uses agent-1, act-B uses agent-2)
		if env.Author == "agent-1" {
			actA = append(actA, env.Type)
		} else if env.Author == "agent-2" {
			actB = append(actB, env.Type)
		}
	}

	// Verify each activity's FIFO order
	expectedSeq := []contract.EnvelopeType{
		contract.EnvelopeTypeActivityStart,
		contract.EnvelopeTypeActivityDelta,
		contract.EnvelopeTypeActivityDone,
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
		env:      contract.NewEnvelope(contract.EnvelopeTypeActivityDone, "agent-1", "sess-1"),
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
			env: contract.NewEnvelope(contract.EnvelopeTypeActivityStart, "agent-1", "sess-1"),
		})
		_ = seq.publish(ctx, activityID, publishTask{
			env: contract.NewEnvelope(contract.EnvelopeTypeActivityDone, "agent-1", "sess-1"),
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
		env: contract.NewEnvelope(contract.EnvelopeTypeActivityStart, "agent-1", "sess-1"),
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
		env: contract.NewEnvelope(contract.EnvelopeTypeActivityDelta, "agent-1", "sess-1"),
	})

	// Wait until the consumer has called bus.Publish (and is now blocked).
	<-bus.publishCalled

	// Consumer is blocked. Fill the channel buffer (defaultChannelBufferSize tasks).
	for i := 0; i < defaultChannelBufferSize; i++ {
		_ = seq.publish(context.Background(), activityID, publishTask{
			env: contract.NewEnvelope(contract.EnvelopeTypeActivityDelta, "agent-1", "sess-1"),
		})
	}

	// Buffer is now full. This publish should block until ctx timeout.
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := seq.publish(ctx, activityID, publishTask{
		env: contract.NewEnvelope(contract.EnvelopeTypeActivityDone, "agent-1", "sess-1"),
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
		env: contract.NewEnvelope(contract.EnvelopeTypeActivityDelta, "agent-1", "sess-1"),
	})

	// Wait until consumer is blocked on bus.Publish
	<-bus.publishCalled

	// Fill the channel buffer
	for i := 0; i < defaultChannelBufferSize; i++ {
		_ = seq.publish(context.Background(), activityID, publishTask{
			env: contract.NewEnvelope(contract.EnvelopeTypeActivityDelta, "agent-1", "sess-1"),
		})
	}

	// Cancel context before publishing
	cancel()
	err := seq.publish(ctx, activityID, publishTask{
		env: contract.NewEnvelope(contract.EnvelopeTypeActivityDone, "agent-1", "sess-1"),
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

// blockingBus is an event.Bus that blocks on Publish until publishCh is closed.
// publishCalled signals when the consumer has entered Publish (and is blocked).
// Used to simulate backpressure for testing.
type blockingBus struct {
	publishCalled chan struct{}
	publishCh     chan struct{}
}

func (b *blockingBus) Publish(_ context.Context, _ contract.Envelope) {
	select {
	case b.publishCalled <- struct{}{}:
	default:
	}
	<-b.publishCh
}

func (b *blockingBus) Subscribe(_ contract.SubscribeOptions) (<-chan contract.Envelope, func()) {
	return nil, func() {}
}

func (b *blockingBus) DropCount() uint64 { return 0 }

// deltaTask creates a publishTask for an activity_delta envelope.
func deltaTask(field, chunk string) publishTask {
	env := contract.NewEnvelope(contract.EnvelopeTypeActivityDelta, "agent-1", "sess-1")
	env.Metadata = map[string]any{
		"delta_field": field,
		"delta_chunk": chunk,
	}
	return publishTask{env: env}
}

// TestActivityEventSequencer_DeltaBatching verifies that consecutive
// activity_delta envelopes for the same field are coalesced into a single
// envelope, reducing frontend event frequency.
func TestActivityEventSequencer_DeltaBatching(t *testing.T) {
	bus := newSyncCaptureBus()
	seq := newActivityEventSequencer(bus, loggateway.NewNoop())
	seq.deltaBatchInterval = 5 * time.Millisecond
	defer seq.Close()

	activityID := "act-batch"
	ctx := context.Background()

	_ = seq.publish(ctx, activityID, publishTask{
		env: contract.NewEnvelope(contract.EnvelopeTypeActivityStart, "agent-1", "sess-1"),
	})
	_ = seq.publish(ctx, activityID, deltaTask("content", "a"))
	_ = seq.publish(ctx, activityID, deltaTask("content", "b"))
	_ = seq.publish(ctx, activityID, deltaTask("content", "c"))
	_ = seq.publish(ctx, activityID, publishTask{
		env: contract.NewEnvelope(contract.EnvelopeTypeActivityDone, "agent-1", "sess-1"),
	})

	envs := bus.waitForPublished(t, 3)

	expected := []contract.EnvelopeType{
		contract.EnvelopeTypeActivityStart,
		contract.EnvelopeTypeActivityDelta,
		contract.EnvelopeTypeActivityDone,
	}
	for i, want := range expected {
		if envs[i].Type != want {
			t.Errorf("envelope[%d] type=%q want %q", i, envs[i].Type, want)
		}
	}
	if envs[1].Metadata["delta_chunk"] != "abc" {
		t.Errorf("batched delta chunk=%q want %q", envs[1].Metadata["delta_chunk"], "abc")
	}
}

// TestActivityEventSequencer_DeltaBatchingDifferentFields verifies that delta
// envelopes for different fields are not merged.
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
		envs[0].Metadata["delta_chunk"].(string),
		envs[1].Metadata["delta_chunk"].(string),
		envs[2].Metadata["delta_chunk"].(string),
	}
	want := []string{"a", "b", "c"}
	for i, w := range want {
		if chunks[i] != w {
			t.Errorf("envelope[%d] chunk=%q want %q", i, chunks[i], w)
		}
	}
}

// TestActivityEventSequencer_DeltaBatchingTimerFlush verifies that a single
// delta envelope is flushed after the batch interval expires.
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
		t.Fatalf("expected 1 envelope, got %d", len(envs))
	}
	if envs[0].Metadata["delta_chunk"] != "x" {
		t.Errorf("chunk=%q want %q", envs[0].Metadata["delta_chunk"], "x")
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
		env: contract.NewEnvelope(contract.EnvelopeTypeActivityDone, "agent-1", "sess-1"),
	})

	envs := bus.waitForPublished(t, 2)
	if envs[0].Type != contract.EnvelopeTypeActivityDelta {
		t.Errorf("first envelope type=%q want activity_delta", envs[0].Type)
	}
	if envs[0].Metadata["delta_chunk"] != "ab" {
		t.Errorf("batched chunk=%q want %q", envs[0].Metadata["delta_chunk"], "ab")
	}
	if envs[1].Type != contract.EnvelopeTypeActivityDone {
		t.Errorf("second envelope type=%q want activity_done", envs[1].Type)
	}
}
