package v2

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// capturingSequencer implements SequencerPublisher and records every Publish call
// in order. Safe for concurrent use.
type capturingSequencer struct {
	mu     sync.Mutex
	events []biz.Event
}

func (c *capturingSequencer) Publish(_ context.Context, e biz.Event) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, e)
}

// drain returns all captured events and clears the buffer. Used between test
// phases to isolate the events emitted by the next phase.
func (c *capturingSequencer) drain() []biz.Event {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]biz.Event, len(c.events))
	copy(out, c.events)
	c.events = c.events[:0]
	return out
}

// newTestProjector builds a projector wired to a capturing sequencer and the
// v2-local defaultSeqAssigner (deterministic 1,2,3... per spirit session).
func newTestProjector(t *testing.T) (*ActivityProjector, *capturingSequencer) {
	t.Helper()
	cap := &capturingSequencer{}
	p := NewActivityProjector(cap, NewDefaultSeqAssigner(), loggateway.NewNoop())
	return p, cap
}

// rootMeta returns a root (non-team) ProjectMeta for testing.
func rootMeta() ProjectMeta {
	return ProjectMeta{
		SessionID:       "sess-1",
		SpiritSessionID: "sess-1",
		TaskID:          "task-1",
		TurnID:          "turn-1",
		AgentKey:        "agent-1",
		AgentName:       "Agent One",
		TaskContent:     "hello user",
	}
}

// TestProjector_OnTurnStart_EmitsTaskAndTurnCreated verifies that a root turn
// (TeamStageID empty) emits both task.created and turn.started, in that order.
func TestProjector_OnTurnStart_EmitsTaskAndTurnCreated(t *testing.T) {
	t.Parallel()
	p, cap := newTestProjector(t)
	meta := rootMeta()

	p.OnTurnStart(context.Background(), meta)

	events := cap.drain()
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d: %+v", len(events), events)
	}
	if events[0].EventKind() != biz.EventKindTaskCreated {
		t.Errorf("event[0] = %s, want %s", events[0].EventKind(), biz.EventKindTaskCreated)
	}
	if events[1].EventKind() != biz.EventKindTurnStarted {
		t.Errorf("event[1] = %s, want %s", events[1].EventKind(), biz.EventKindTurnStarted)
	}
	// Verify the task carries the user message and running status.
	tc := events[0].(*biz.TaskCreatedEvent)
	if tc.Task.UserMessage != "hello user" {
		t.Errorf("task.UserMessage = %q, want %q", tc.Task.UserMessage, "hello user")
	}
	if tc.Task.Status != biz.TaskStatusRunning {
		t.Errorf("task.Status = %q, want %q", tc.Task.Status, biz.TaskStatusRunning)
	}
	// Verify the turn references the task and got a non-zero Seq.
	ts := events[1].(*biz.TurnStartedEvent)
	if ts.Turn.TaskID != "task-1" {
		t.Errorf("turn.TaskID = %q, want %q", ts.Turn.TaskID, "task-1")
	}
	if ts.Turn.Seq == 0 {
		t.Errorf("turn.Seq = 0, want non-zero")
	}
}

// TestProjector_OnReasoningDelta_EmitsStepStreaming verifies that streaming a
// reasoning delta emits a step.streaming event with DeltaField=reasoning.
func TestProjector_OnReasoningDelta_EmitsStepStreaming(t *testing.T) {
	t.Parallel()
	p, cap := newTestProjector(t)
	meta := rootMeta()
	ctx := context.Background()

	p.OnTurnStart(ctx, meta)
	cap.drain() // drain setup events

	stepID := p.BeginStep(meta, biz.StepKindThinking)
	p.OnReasoningDelta(ctx, stepID, "thinking...", "")

	events := cap.drain()
	if len(events) != 2 {
		t.Fatalf("expected 2 events (created + streaming), got %d", len(events))
	}
	if events[0].EventKind() != biz.EventKindStepCreated {
		t.Errorf("event[0] = %s, want %s", events[0].EventKind(), biz.EventKindStepCreated)
	}
	if events[1].EventKind() != biz.EventKindStepStreaming {
		t.Fatalf("event[1] = %s, want %s", events[1].EventKind(), biz.EventKindStepStreaming)
	}
	ss := events[1].(*biz.StepStreamingEvent)
	if ss.DeltaField != "reasoning" {
		t.Errorf("DeltaField = %q, want %q", ss.DeltaField, "reasoning")
	}
	if ss.DeltaChunk != "thinking..." {
		t.Errorf("DeltaChunk = %q, want %q", ss.DeltaChunk, "thinking...")
	}
	if ss.StepID != stepID {
		t.Errorf("StepID = %q, want %q", ss.StepID, stepID)
	}
}

// TestProjector_OnTextDeltaThenDone_CompletesReplyStep verifies that streaming
// text deltas followed by OnTextDone emits a step.completed carrying the full
// content.
func TestProjector_OnTextDeltaThenDone_CompletesReplyStep(t *testing.T) {
	t.Parallel()
	p, cap := newTestProjector(t)
	meta := rootMeta()
	ctx := context.Background()

	p.OnTurnStart(ctx, meta)
	cap.drain() // drain setup events

	stepID := p.BeginStep(meta, biz.StepKindReply)
	p.OnTextDelta(ctx, stepID, "hello ", "")
	p.OnTextDelta(ctx, stepID, "world", "")
	cap.drain() // drain created + 2 streaming
	p.OnTextDone(ctx, stepID, "hello world", true)

	events := cap.drain()
	if len(events) != 1 {
		t.Fatalf("expected 1 event (completed), got %d", len(events))
	}
	if events[0].EventKind() != biz.EventKindStepCompleted {
		t.Fatalf("event = %s, want %s", events[0].EventKind(), biz.EventKindStepCompleted)
	}
	sc := events[0].(*biz.StepCompletedEvent)
	if sc.Step.Content != "hello world" {
		t.Errorf("step.Content = %q, want %q", sc.Step.Content, "hello world")
	}
	if !sc.Step.IsFinal {
		t.Errorf("step.IsFinal = false, want true")
	}
	if sc.Step.Status != biz.StepStatusCompleted {
		t.Errorf("step.Status = %q, want %q", sc.Step.Status, biz.StepStatusCompleted)
	}
}

// TestProjector_OnToolCall_EmitsStepCreatedThenUpdated verifies that a tool call
// on a fresh turn emits step.created (action) followed by step.updated carrying
// the tool name.
func TestProjector_OnToolCall_EmitsStepCreatedThenUpdated(t *testing.T) {
	t.Parallel()
	p, cap := newTestProjector(t)
	meta := rootMeta()
	ctx := context.Background()
	args := json.RawMessage(`{"q":"gopher"}`)

	p.OnTurnStart(ctx, meta)
	cap.drain() // drain setup events

	p.OnToolCall(ctx, meta, "search", args)

	events := cap.drain()
	if len(events) != 2 {
		t.Fatalf("expected 2 events (created + updated), got %d", len(events))
	}
	if events[0].EventKind() != biz.EventKindStepCreated {
		t.Errorf("event[0] = %s, want %s", events[0].EventKind(), biz.EventKindStepCreated)
	}
	if events[1].EventKind() != biz.EventKindStepUpdated {
		t.Errorf("event[1] = %s, want %s", events[1].EventKind(), biz.EventKindStepUpdated)
	}
	su := events[1].(*biz.StepUpdatedEvent)
	if su.Step.ToolName != "search" {
		t.Errorf("step.ToolName = %q, want %q", su.Step.ToolName, "search")
	}
	if su.Step.Status != biz.StepStatusToolRunning {
		t.Errorf("step.Status = %q, want %q", su.Step.Status, biz.StepStatusToolRunning)
	}
}

// TestProjector_OnTurnEnd_RootEmitsTaskCompleted verifies that ending a root
// turn emits both turn.completed and task.completed.
func TestProjector_OnTurnEnd_RootEmitsTaskCompleted(t *testing.T) {
	t.Parallel()
	p, cap := newTestProjector(t)
	meta := rootMeta()
	ctx := context.Background()

	p.OnTurnStart(ctx, meta)
	cap.drain() // drain setup events
	p.OnTurnEnd(ctx, meta)

	events := cap.drain()
	if len(events) != 2 {
		t.Fatalf("expected 2 events (turn.completed + task.completed), got %d", len(events))
	}
	if events[0].EventKind() != biz.EventKindTurnCompleted {
		t.Errorf("event[0] = %s, want %s", events[0].EventKind(), biz.EventKindTurnCompleted)
	}
	if events[1].EventKind() != biz.EventKindTaskCompleted {
		t.Errorf("event[1] = %s, want %s", events[1].EventKind(), biz.EventKindTaskCompleted)
	}
	tc := events[1].(*biz.TaskCompletedEvent)
	if tc.Task.Status != biz.TaskStatusCompleted {
		t.Errorf("task.Status = %q, want %q", tc.Task.Status, biz.TaskStatusCompleted)
	}
	if tc.Task.CompletedAt == nil {
		t.Errorf("task.CompletedAt = nil, want set")
	}
}
