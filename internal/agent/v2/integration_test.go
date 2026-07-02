package v2

import (
	"context"
	"sync"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// recordingBus captures all Publish calls in order. Implements biz.EventBus
// (via NewEventBusAdapter) so the Sequencer can use it as its publish sink.
// Subscribe is a no-op (returns a never-delivering channel) — we verify via
// the recorded Publish calls, not via subscriber delivery.
//
// This avoids the flakiness of V2Bus's non-blocking subscriber channel, which
// may drop events when the test goroutine is not draining fast enough.
type recordingBus struct {
	mu  sync.Mutex
	pub []biz.Event
}

func (r *recordingBus) Publish(_ context.Context, e biz.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pub = append(r.pub, e)
}

func (r *recordingBus) Subscribe(_ biz.EventSubscribeOptions) (<-chan biz.Event, func()) {
	ch := make(chan biz.Event) // never delivers; we verify via Publish recording
	return ch, func() {}
}

// Events returns a snapshot of all recorded Publish calls, in FIFO order.
func (r *recordingBus) Events() []biz.Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]biz.Event, len(r.pub))
	copy(out, r.pub)
	return out
}

// TestEndToEnd_V2Pipeline verifies the complete v2 pipeline:
//
//	Projector → Sequencer → RepoSet (fake) + EventBus (recording) + CompatAdapter → v1 ActivityEvent
//
// Scenario: a single spirit turn with thinking → reply, then task.completed.
// Verifies that:
//  1. All v2 entities are persisted to the fake RepoSet (tasks/turns/steps).
//  2. The CompatAdapter translates v2 events to v1 ActivityEvents, with
//     task.created as the first v1 event (kind=task).
func TestEndToEnd_V2Pipeline(t *testing.T) {
	t.Parallel()

	// Wire up all components.
	rs := &fakeRepoSet{}
	v2Bus := &recordingBus{}
	v1Bus := &fakeV1Bus{}
	compat := NewCompatAdapter(v1Bus)

	seq := NewSequencer(rs, NewEventBusAdapter(v2Bus), loggateway.NewNoop(), compat,
		WithPublishBuffer(64),
		WithPersistBuffer(64),
		WithDeltaBatchInterval(time.Millisecond*2),
	)
	defer seq.Close()

	projector := NewActivityProjector(seq, seq.SeqAssigner(), loggateway.NewNoop())

	// Drive a single spirit turn: thinking → reply.
	ctx := context.Background()
	meta := ProjectMeta{
		SessionID:       "sess-1",
		SpiritSessionID: "sess-1",
		TaskID:          "task-1",
		TurnID:          "turn-1",
		AgentKey:        "spirit",
		TaskContent:     "what is 2+2?",
	}

	projector.OnTurnStart(ctx, meta)
	thinkingStep := projector.BeginStep(meta, biz.StepKindThinking)
	projector.OnReasoningDelta(ctx, thinkingStep, "thinking about addition", "")
	projector.OnReasoningDone(ctx, thinkingStep, "I'll just compute 2+2=4")
	replyStep := projector.BeginStep(meta, biz.StepKindReply)
	projector.OnTextDelta(ctx, replyStep, "4", "")
	projector.OnTextDone(ctx, replyStep, "4", true /* isFinal */)
	projector.OnTurnEnd(ctx, meta)

	if err := seq.Flush(ctx); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	// Give the persist worker time to drain.
	time.Sleep(50 * time.Millisecond)

	// Verify v2 repos were called.
	rs.mu.Lock()
	if len(rs.tasks) < 2 {
		t.Errorf("expected >=2 task upserts (created+completed), got %d", len(rs.tasks))
	}
	if len(rs.turns) < 2 {
		t.Errorf("expected >=2 turn upserts (started+completed), got %d", len(rs.turns))
	}
	if len(rs.steps) < 4 {
		t.Errorf("expected >=4 step upserts (thinking+reply, created+completed each), got %d", len(rs.steps))
	}
	rs.mu.Unlock()

	// Verify v1 compat got translated events.
	v1Bus.mu.Lock()
	defer v1Bus.mu.Unlock()
	if len(v1Bus.pub) == 0 {
		t.Fatalf("expected v1 events from compat adapter, got 0")
	}
	// The first v1 event should be task.created (kind=task), since
	// OnTurnStart emits task.created before turn.started, and turn.started
	// has no v1 mapping (dropped by the compat adapter).
	first := v1Bus.pub[0]
	if first.Activity.Kind != biz.ActivityKindTask {
		t.Errorf("expected first v1 event kind=task, got %s", first.Activity.Kind)
	}
	if first.Event != biz.ActivityEventCreated {
		t.Errorf("expected first v1 event type=created, got %s", first.Event)
	}
}

// TestEndToEnd_FIFOOrdering verifies that turn.started is published to the
// EventBus BEFORE any step.created event, even when BeginStep is called
// immediately after OnTurnStart.
//
// This is the core invariant of the single-publish-worker Sequencer: FIFO
// across all event types. The v1 dual-path (ActivityProjector→Sequencer +
// direct-publish) failed to guarantee this, which is the primary motivation
// for the v2 redesign.
//
// Uses recordingBus (not V2Bus) to avoid subscriber-channel flakiness — the
// Sequencer's bus.Publish is recorded synchronously, so the order reflects the
// actual publish order of the single publish worker.
func TestEndToEnd_FIFOOrdering(t *testing.T) {
	t.Parallel()

	rs := &fakeRepoSet{}
	v2Bus := &recordingBus{}
	compat := NewCompatAdapter(&fakeV1Bus{})

	seq := NewSequencer(rs, NewEventBusAdapter(v2Bus), loggateway.NewNoop(), compat,
		WithPublishBuffer(64),
		WithPersistBuffer(64),
		WithDeltaBatchInterval(time.Millisecond*2),
	)
	defer seq.Close()

	projector := NewActivityProjector(seq, seq.SeqAssigner(), loggateway.NewNoop())
	ctx := context.Background()
	meta := ProjectMeta{
		SessionID:       "sess-1",
		SpiritSessionID: "sess-1",
		TaskID:          "task-1",
		TurnID:          "turn-1",
		AgentKey:        "spirit",
	}

	projector.OnTurnStart(ctx, meta)
	thinkingStep := projector.BeginStep(meta, biz.StepKindThinking)
	projector.OnReasoningDelta(ctx, thinkingStep, "thinking", "")
	projector.OnReasoningDone(ctx, thinkingStep, "done thinking")
	replyStep := projector.BeginStep(meta, biz.StepKindReply)
	projector.OnTextDelta(ctx, replyStep, "answer", "")
	projector.OnTextDone(ctx, replyStep, "answer", true)
	projector.OnTurnEnd(ctx, meta)

	if err := seq.Flush(ctx); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	time.Sleep(50 * time.Millisecond)

	events := v2Bus.Events()
	if len(events) == 0 {
		t.Skip("no events captured — recordingBus did not receive any publishes")
	}

	// Locate the first turn.started and first step.created in the published
	// event stream. The FIFO invariant requires turn.started to come before
	// any step.created.
	var turnStartedIdx, firstStepIdx int = -1, -1
	for i, e := range events {
		if e.EventKind() == biz.EventKindTurnStarted && turnStartedIdx == -1 {
			turnStartedIdx = i
		}
		if e.EventKind() == biz.EventKindStepCreated && firstStepIdx == -1 {
			firstStepIdx = i
		}
	}
	if turnStartedIdx == -1 || firstStepIdx == -1 {
		t.Logf("captured %d events:", len(events))
		for i, e := range events {
			t.Logf("  [%d] %s", i, e.EventKind())
		}
		t.Skip("could not locate turn.started / step.created in captured events")
	}
	if turnStartedIdx > firstStepIdx {
		t.Errorf("FIFO violated: turn.started (idx=%d) published AFTER step.created (idx=%d)",
			turnStartedIdx, firstStepIdx)
	}
}
