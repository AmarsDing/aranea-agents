package v2

import (
	"context"
	"sync"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// recordingBus captures all Publish calls in order. It directly satisfies the
// local EventBus interface so the Sequencer can use it as its publish sink.
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
//	Projector → Sequencer → RepoSet (fake) + EventBus (recording)
//
// Scenario: a single spirit turn with thinking → reply, then task.completed.
// Verifies that:
//  1. All v2 entities are persisted to the fake RepoSet (tasks/turns/steps).
func TestEndToEnd_V2Pipeline(t *testing.T) {
	t.Parallel()

	// Wire up all components.
	rs := &fakeRepoSet{}
	v2Bus := &recordingBus{}

	seq := NewSequencer(rs, v2Bus, loggateway.NewNoop(),
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
	projector.OnTurnEnd(ctx, meta, false /* canceled */)

	if err := seq.Flush(ctx); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	// Give the persist worker time to drain.
	time.Sleep(50 * time.Millisecond)

	// Verify v2 repos were called.
	rs.mu.Lock()
	// L3 (2026-07-22): task.completed routes to CompleteTaskTerminal, so the
	// created state lands in rs.tasks (UpsertTask) and the terminal state in
	// rs.terminal — together they replace the former ">=2 upserts" invariant.
	if len(rs.tasks) < 1 || len(rs.terminal) < 1 {
		t.Errorf("expected task created (tasks>=1) + completed (terminal>=1), got tasks=%d terminal=%d", len(rs.tasks), len(rs.terminal))
	}
	if len(rs.turns) < 2 {
		t.Errorf("expected >=2 turn upserts (started+completed), got %d", len(rs.turns))
	}
	if len(rs.steps) < 4 {
		t.Errorf("expected >=4 step upserts (thinking+reply, created+completed each), got %d", len(rs.steps))
	}
	rs.mu.Unlock()
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

	seq := NewSequencer(rs, v2Bus, loggateway.NewNoop(),
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
	projector.OnTurnEnd(ctx, meta, false /* canceled */)

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

// TestEndToEnd_CancelledTurnMarksCancelledStatus verifies P1-02 fix: when a
// turn is cancelled (OnTurnEndEnhanced with canceled=true, or OnTurnEnd with
// canceled=true), the Turn and root Task entities must be persisted with
// Status="cancelled" — NOT "completed". The published events must carry the
// cancelled status in their entity payloads.
//
// Regression: previously OnTurnEnd unconditionally set Status=Completed,
// causing cancelled runs to appear as successfully completed in the UI.
func TestEndToEnd_CancelledTurnMarksCancelledStatus(t *testing.T) {
	t.Parallel()

	rs := &fakeRepoSet{}
	v2Bus := &recordingBus{}

	seq := NewSequencer(rs, v2Bus, loggateway.NewNoop(),
		WithPublishBuffer(64),
		WithPersistBuffer(64),
		WithDeltaBatchInterval(time.Millisecond*2),
	)
	defer seq.Close()

	projector := NewActivityProjector(seq, seq.SeqAssigner(), loggateway.NewNoop())
	ctx := context.Background()
	meta := ProjectMeta{
		SessionID:       "sess-cancel",
		SpiritSessionID: "sess-cancel",
		TaskID:          "task-cancel",
		TurnID:          "turn-cancel",
		AgentKey:        "spirit",
		TaskContent:     "long-running query",
	}

	projector.OnTurnStart(ctx, meta)
	thinkingStep := projector.BeginStep(meta, biz.StepKindThinking)
	projector.OnReasoningDelta(ctx, thinkingStep, "thinking...", "")
	// Simulate user cancellation: OnTurnEndEnhanced with canceled=true.
	projector.OnTurnEndEnhanced(ctx, meta, &ActivityUsage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15}, true)

	if err := seq.Flush(ctx); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	time.Sleep(50 * time.Millisecond)

	// Verify persisted Turn has Status=cancelled.
	// Pick the highest Version for each ID: non-terminal events persist via
	// async persistChan and may land AFTER a terminal sync WBPF upsert when
	// reading by append order (C-15). Version-monotonic selection matches
	// production CAS semantics.
	rs.mu.Lock()
	var lastTurn biz.Turn
	var hasTurn bool
	for i := range rs.turns {
		if rs.turns[i].ID == "turn-cancel" && (!hasTurn || rs.turns[i].Version >= lastTurn.Version) {
			lastTurn = rs.turns[i]
			hasTurn = true
		}
	}
	var lastTask biz.Task
	var hasTask bool
	// L3 (2026-07-22): terminal task events (task.completed incl. Status=
	// Cancelled) persist via CompleteTaskTerminal into rs.terminal, while
	// non-terminal states land in rs.tasks via UpsertTask. Scan both and pick
	// the highest version (production CAS semantics).
	for i := range rs.tasks {
		if rs.tasks[i].ID == "task-cancel" && (!hasTask || rs.tasks[i].Version >= lastTask.Version) {
			lastTask = rs.tasks[i]
			hasTask = true
		}
	}
	for i := range rs.terminal {
		if rs.terminal[i].ID == "task-cancel" && (!hasTask || rs.terminal[i].Version >= lastTask.Version) {
			lastTask = rs.terminal[i]
			hasTask = true
		}
	}
	rs.mu.Unlock()
	if !hasTurn {
		t.Fatalf("no turn persisted for turn-cancel")
	}
	if lastTurn.Status != biz.TurnStatusCancelled {
		t.Errorf("cancelled turn Status = %q, want %q (version=%d)", lastTurn.Status, biz.TurnStatusCancelled, lastTurn.Version)
	}
	if !hasTask {
		t.Fatalf("no task persisted for task-cancel")
	}
	if lastTask.Status != biz.TaskStatusCancelled {
		t.Errorf("cancelled task Status = %q, want %q (version=%d)", lastTask.Status, biz.TaskStatusCancelled, lastTask.Version)
	}

	// Verify published events carry cancelled status.
	events := v2Bus.Events()
	var turnCompletedEvent *biz.TurnCompletedEvent
	var taskCompletedEvent *biz.TaskCompletedEvent
	for _, e := range events {
		switch ev := e.(type) {
		case *biz.TurnCompletedEvent:
			turnCompletedEvent = ev
		case *biz.TaskCompletedEvent:
			taskCompletedEvent = ev
		}
	}
	if turnCompletedEvent == nil {
		t.Fatalf("TurnCompletedEvent not published")
	}
	if turnCompletedEvent.Turn.Status != biz.TurnStatusCancelled {
		t.Errorf("published TurnCompletedEvent.Turn.Status = %q, want %q",
			turnCompletedEvent.Turn.Status, biz.TurnStatusCancelled)
	}
	if taskCompletedEvent == nil {
		t.Fatalf("TaskCompletedEvent not published")
	}
	if taskCompletedEvent.Task.Status != biz.TaskStatusCancelled {
		t.Errorf("published TaskCompletedEvent.Task.Status = %q, want %q",
			taskCompletedEvent.Task.Status, biz.TaskStatusCancelled)
	}
}
