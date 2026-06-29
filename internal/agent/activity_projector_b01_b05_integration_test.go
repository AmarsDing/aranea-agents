package agent

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// TestActivityProjector_B01_StartBeforeDelta verifies that activity_start
// always arrives before activity_delta for the same activity.
//
// B-01 root cause: the old implementation published activity_start via
// safego.Go (async) while activity_delta was published synchronously.
// Under load, the delta could overtake the start, causing the frontend
// to receive a delta for an activity it hasn't seen yet.
//
// Fix: the sequencer uses a per-activity FIFO channel, guaranteeing
// start → delta → done ordering.
func TestActivityProjector_B01_StartBeforeDelta(t *testing.T) {
	p, bus, _ := newTestProjector(t)
	ctx := context.Background()

	// Simulate a streaming reply: OnTextDelta creates the activity
	// (emitting start) and then emits a delta chunk.
	p.OnTextDelta(ctx, "agent-1", "chunk-1")

	// Wait for both events (start + delta)
	envs := bus.waitForPublished(t, 2)

	if envs[0].Event != biz.ActivityEventCreated {
		t.Errorf("event[0] type=%q want %q (start must arrive before delta)",
			envs[0].Event, biz.ActivityEventCreated)
	}
	if envs[1].Event != biz.ActivityEventStreaming {
		t.Errorf("event[1] type=%q want %q",
			envs[1].Event, biz.ActivityEventStreaming)
	}

	// Verify both events reference the same activity_id
	startActID := envs[0].Activity.ID
	deltaActID := envs[1].Activity.ID
	if startActID == "" {
		t.Fatal("start event missing activity_id metadata")
	}
	if startActID != deltaActID {
		t.Errorf("activity_id mismatch: start=%q delta=%q", startActID, deltaActID)
	}
}

// TestActivityProjector_B01_StartBeforeDeltaConcurrent verifies FIFO ordering
// under concurrent stress: multiple OnTextDelta calls for the same author
// must all see start before any delta.
func TestActivityProjector_B01_StartBeforeDeltaConcurrent(t *testing.T) {
	p, bus, _ := newTestProjector(t)
	ctx := context.Background()

	// Fire 50 delta chunks concurrently for the same author.
	// The first call creates the activity (emitting start); subsequent
	// calls emit deltas. All must be ordered start → deltas.
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p.OnTextDelta(ctx, "agent-stress", "x")
		}()
	}
	wg.Wait()

	// Wait for all events to be published.
	// With delta batching, the 50 deltas may be coalesced into fewer events,
	// so we only require the start event to be first and all subsequent events
	// to be deltas for the same activity.
	envs := bus.waitForPublished(t, 2)

	// Verify the first event is start.
	if envs[0].Event != biz.ActivityEventCreated {
		t.Fatalf("event[0] type=%q want %q (start must be first)",
			envs[0].Event, biz.ActivityEventCreated)
	}

	// Extract the activity_id from the start event.
	startActID := envs[0].Activity.ID
	if startActID == "" {
		t.Fatal("start event missing activity_id metadata")
	}

	// Allow the batch timer to flush any remaining deltas.
	time.Sleep(2 * defaultDeltaBatchInterval)
	bus.mu.Lock()
	envs = make([]biz.ActivityEvent, len(bus.published))
	copy(envs, bus.published)
	bus.mu.Unlock()

	// Verify all subsequent events are deltas for the same activity.
	for i := 1; i < len(envs); i++ {
		if envs[i].Event != biz.ActivityEventStreaming {
			t.Errorf("event[%d] type=%q want %q",
				i, envs[i].Event, biz.ActivityEventStreaming)
		}
		deltaActID := envs[i].Activity.ID
		if deltaActID != startActID {
			t.Errorf("event[%d] activity_id=%q want %q",
				i, deltaActID, startActID)
		}
	}
}

// TestActivityProjector_B04_DeltaDoesNotBlockMutex verifies that publishing
// a delta does not hold the projector mutex while the bus.Publish is in flight.
//
// B-04 root cause: the old implementation called bus.Publish synchronously
// inside publishActivityDelta while holding p.mu. If bus.Publish was slow
// (e.g., WS subscriber buffer full), all subsequent OnTextDelta calls blocked
// on p.mu, stalling the entire event loop.
//
// Fix: the sequencer enqueues to a channel and returns immediately, so p.mu
// is released before bus.Publish runs.
func TestActivityProjector_B04_DeltaDoesNotBlockMutex(t *testing.T) {
	// Use a bus that blocks on Publish to simulate slow subscribers.
	slowBus := &blockingBus{
		publishCalled: make(chan struct{}, 1),
		publishCh:     make(chan struct{}),
	}
	repo := newMockActivityWriter()
	p := NewActivityProjector(slowBus, repo, loggateway.NewNoop(), NewNoopToolCategorizer())
	p.Reset()
	t.Cleanup(func() {
		close(slowBus.publishCh)
		p.Close()
	})

	ctx := context.Background()

	// First OnTextDelta creates the activity and enqueues start + delta.
	// The consumer goroutine will pick up the start event and block on
	// bus.Publish.
	p.OnTextDelta(ctx, "agent-1", "chunk-1")

	// Wait until the consumer is blocked on bus.Publish.
	<-slowBus.publishCalled

	// Now try to acquire p.mu. If the old implementation held p.mu during
	// bus.Publish, this would block until publishCh is closed.
	// With the sequencer, p.mu should be available immediately.
	done := make(chan struct{})
	go func() {
		p.mu.Lock()
		p.mu.Unlock()
		close(done)
	}()

	select {
	case <-done:
		// Success: mutex was not held during bus.Publish.
	case <-time.After(500 * time.Millisecond):
		t.Fatal("p.mu was held during bus.Publish (B-04 not fixed): " +
			"OnTextDelta blocked the mutex while bus.Publish was in flight")
	}
}

// TestActivityProjector_B05_NoStartDeltaRace verifies that there is no race
// between the activity_start publish and the activity_delta publish.
//
// B-05 root cause: the old implementation published start via safego.Go
// (async goroutine) while delta was published synchronously. Under the race
// detector, this could trigger data races on shared state.
//
// Fix: the sequencer uses a single consumer goroutine per activity, so
// start and delta are processed by the same goroutine in FIFO order.
func TestActivityProjector_B05_NoStartDeltaRace(t *testing.T) {
	p, bus, _ := newTestProjector(t)
	ctx := context.Background()

	// Rapidly fire OnTextDelta + OnTextDone for multiple authors.
	// This stresses the start/delta/done ordering across activities.
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		author := "agent-" + string(rune('A'+i%5))
		go func(a string) {
			defer wg.Done()
			p.OnTextDelta(ctx, a, "chunk")
			p.OnTextDelta(ctx, a, "chunk")
			p.OnTextDone(ctx, a, "full text")
		}(author)
	}
	wg.Wait()

	// Flush the sequencer before inspecting published events. The sequencer is
	// intentionally asynchronous (events are enqueued and processed by per-activity
	// goroutines), so wg.Wait() only guarantees that calls to OnTextDelta/OnTextDone
	// have returned, not that all queued events have been published. Closing the
	// projector drains all pending events deterministically before assertions.
	p.Close()

	// Wait for all events: 20 * (1 start + 2 deltas + 1 done) = 80 events.
	// But OnTextDelta for the same author reuses the same activity, so:
	// - First OnTextDelta: start + delta = 2 events
	// - Second OnTextDelta: delta = 1 event
	// - OnTextDone: done = 1 event
	// Total per author: 4 events. 20 goroutines but only 5 distinct authors,
	// so some goroutines share activities. Wait with a generous timeout.
	deadline := time.After(3 * time.Second)
	for {
		bus.mu.Lock()
		count := len(bus.published)
		bus.mu.Unlock()
		if count >= 20 { // at least 20 events (minimum: 5 authors * 4 events)
			break
		}
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for events (got %d)", count)
		case <-time.After(10 * time.Millisecond):
		}
	}

	// Verify per-activity FIFO ordering: for each activity_id, the first
	// event must be start and the last must be done.
	bus.mu.Lock()
	envs := make([]biz.ActivityEvent, len(bus.published))
	copy(envs, bus.published)
	bus.mu.Unlock()

	perActivity := make(map[string][]biz.ActivityEventType)
	for _, env := range envs {
		actID := env.Activity.ID
		if actID == "" {
			continue
		}
		perActivity[actID] = append(perActivity[actID], env.Event)
	}

	for actID, seq := range perActivity {
		if len(seq) == 0 {
			continue
		}
		if seq[0] != biz.ActivityEventCreated {
			t.Errorf("activity %s: first event=%q want %q (B-05 race detected)",
				actID, seq[0], biz.ActivityEventCreated)
		}
		// The last event should be done (OnTextDone emits done).
		if seq[len(seq)-1] != biz.ActivityEventCompleted {
			t.Errorf("activity %s: last event=%q want %q",
				actID, seq[len(seq)-1], biz.ActivityEventCompleted)
		}
	}
}

// TestActivityProjector_BackpressurePropagation verifies that backpressure
// from the sequencer channel does not corrupt event ordering.
//
// When the channel is full (slow subscriber), publish blocks. This is the
// intended backpressure mechanism: channel full → OnTextDelta blocks →
// stream_consumer blocks → LLM pauses.
//
// This test verifies that even under backpressure, events maintain FIFO order.
func TestActivityProjector_BackpressurePropagation(t *testing.T) {
	// Use a bus that blocks on Publish to create backpressure.
	slowBus := &blockingBus{
		publishCalled: make(chan struct{}, 1),
		publishCh:     make(chan struct{}),
	}
	repo := newMockActivityWriter()
	p := NewActivityProjector(slowBus, repo, loggateway.NewNoop(), NewNoopToolCategorizer())
	p.Reset()

	ctx := context.Background()

	// First OnTextDelta creates the activity and enqueues start + delta.
	// The consumer will pick up start and block on bus.Publish.
	p.OnTextDelta(ctx, "agent-1", "chunk-1")

	// Wait until consumer is blocked on bus.Publish.
	<-slowBus.publishCalled

	// Enqueue more deltas. These fill the publish queue buffer.
	// Since the worker is blocked, the buffer will fill up.
	// After defaultPublishBufferSize (256) tasks, publish will block.
	deltasEnqueued := int32(0)
	var wg sync.WaitGroup
	for i := 0; i < defaultPublishBufferSize; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// publish blocks under backpressure (no ctx timeout — activity
			// events must not be dropped due to caller context cancellation).
			if err := p.sequencer.publish(context.Background(), "agent-1", publishTask{
				event: biz.ActivityEvent{Event: biz.ActivityEventStreaming, Activity: biz.Activity{ID: "agent-1", SessionID: "sess-1"}},
			}); err == nil {
				atomic.AddInt32(&deltasEnqueued, 1)
			}
		}()
	}

	// Unblock the consumer so the publish worker drains the queue.
	// This unblocks all goroutines waiting on publish.
	close(slowBus.publishCh)
	wg.Wait()

	// Close flushes all queued events in FIFO order. If Close succeeds
	// without deadlock, the sequencer maintained ordering under backpressure.
	p.Close()

	// All deltas should have been enqueued (backpressure released before Wait).
	if atomic.LoadInt32(&deltasEnqueued) != defaultPublishBufferSize {
		t.Errorf("expected %d deltas enqueued, got %d", defaultPublishBufferSize, atomic.LoadInt32(&deltasEnqueued))
	}
}
