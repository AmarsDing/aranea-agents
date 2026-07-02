package v2

import (
	"context"
	"sync"
	"testing"

	"aranea-agents/internal/biz"
)

// fakeV1Bus captures v1 ActivityEvents emitted by the CompatAdapter.
// Implements biz.ActivityEventBus (with DropCount per Deviation 9).
type fakeV1Bus struct {
	mu  sync.Mutex
	pub []biz.ActivityEvent
}

func (f *fakeV1Bus) Publish(_ context.Context, e biz.ActivityEvent) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pub = append(f.pub, e)
}

func (f *fakeV1Bus) Subscribe(_ biz.ActivityEventSubscribeOptions) (<-chan biz.ActivityEvent, func()) {
	return make(chan biz.ActivityEvent), func() {}
}

func (f *fakeV1Bus) DropCount() uint64 { return 0 }

// TestCompatAdapter_TaskCreatedToActivityEvent verifies that a v2
// TaskCreatedEvent maps to a v1 ActivityEvent with Event=Created,
// Activity.Kind=task.
func TestCompatAdapter_TaskCreatedToActivityEvent(t *testing.T) {
	t.Parallel()
	v1 := &fakeV1Bus{}
	adapter := NewCompatAdapter(v1)

	// Deviation 1: use factory (unexported fields). Task uses SessionID (= spirit_session_id).
	adapter.PublishV1(context.Background(), biz.NewTaskCreatedEvent(biz.Task{
		ID:        "task-1",
		SessionID: "sess-1",
		Status:    biz.TaskStatusRunning,
		Version:   1,
	}))

	v1.mu.Lock()
	defer v1.mu.Unlock()
	if len(v1.pub) != 1 {
		t.Fatalf("expected 1 v1 event, got %d", len(v1.pub))
	}
	ev := v1.pub[0]
	// Deviation 6: ActivityEvent uses Event field (not Type)
	if ev.Event != biz.ActivityEventCreated {
		t.Errorf("expected v1 event type Created, got %s", ev.Event)
	}
	if ev.Activity.Kind != biz.ActivityKindTask {
		t.Errorf("expected v1 kind task, got %s", ev.Activity.Kind)
	}
	if ev.Activity.ID != "task-1" {
		t.Errorf("expected v1 activity id task-1, got %s", ev.Activity.ID)
	}
	if ev.Activity.SessionID != "sess-1" {
		t.Errorf("expected v1 session id sess-1, got %s", ev.Activity.SessionID)
	}
}

// TestCompatAdapter_StepStreamingToActivityEvent verifies that a v2
// StepStreamingEvent maps to a v1 ActivityEvent with Event=Streaming,
// Kind derived from DeltaField, DeltaChunk carried in the event.
func TestCompatAdapter_StepStreamingToActivityEvent(t *testing.T) {
	t.Parallel()
	v1 := &fakeV1Bus{}
	adapter := NewCompatAdapter(v1)

	// Deviation 2: factory takes (spiritSessionID, taskID, stepID, deltaField, deltaChunk).
	adapter.PublishV1(context.Background(), biz.NewStepStreamingEvent(
		"sess-1", "task-1", "step-1", "content", "hello",
	))

	v1.mu.Lock()
	defer v1.mu.Unlock()
	if len(v1.pub) != 1 {
		t.Fatalf("expected 1 v1 event, got %d", len(v1.pub))
	}
	ev := v1.pub[0]
	if ev.Event != biz.ActivityEventStreaming {
		t.Errorf("expected v1 type Streaming, got %s", ev.Event)
	}
	if ev.Activity.Kind != biz.ActivityKindReply {
		t.Errorf("expected v1 kind reply (content→reply), got %s", ev.Activity.Kind)
	}
	// Deviation 2: DeltaChunk field carries the chunk
	if ev.DeltaChunk != "hello" {
		t.Errorf("expected DeltaChunk hello, got %s", ev.DeltaChunk)
	}
	if ev.DeltaField != "content" {
		t.Errorf("expected DeltaField content, got %s", ev.DeltaField)
	}
}

// TestCompatAdapter_PlanStepStartedToActivityEvent verifies that a v2
// PlanStepStartedEvent maps to a v1 ActivityEvent with Kind=plan.
// Deviation 4: PlanStep uses PlanID (not PlanBoardID).
func TestCompatAdapter_PlanStepStartedToActivityEvent(t *testing.T) {
	t.Parallel()
	v1 := &fakeV1Bus{}
	adapter := NewCompatAdapter(v1)

	// Deviation 4: PlanStep has PlanID field. Factory takes (ps, spiritSessionID).
	adapter.PublishV1(context.Background(), biz.NewPlanStepStartedEvent(biz.PlanStep{
		ID:     "ps-1",
		PlanID: "pb-1",
		TaskID: "task-1",
		Status: biz.PlanStepStatusRunning,
	}, "sess-1"))

	v1.mu.Lock()
	defer v1.mu.Unlock()
	if len(v1.pub) != 1 {
		t.Fatalf("expected 1 v1 event, got %d", len(v1.pub))
	}
	ev := v1.pub[0]
	if ev.Activity.Kind != biz.ActivityKindPlan {
		t.Errorf("expected v1 kind plan, got %s", ev.Activity.Kind)
	}
	if ev.Activity.ID != "ps-1" {
		t.Errorf("expected v1 activity id ps-1, got %s", ev.Activity.ID)
	}
}

// TestCompatAdapter_StepCreatedToActivityEvent verifies step.created maps to
// v1 created event with AuthorAgentKey carried through (Deviation 5).
func TestCompatAdapter_StepCreatedToActivityEvent(t *testing.T) {
	t.Parallel()
	v1 := &fakeV1Bus{}
	adapter := NewCompatAdapter(v1)

	adapter.PublishV1(context.Background(), biz.NewStepCreatedEvent(biz.Step{
		ID:             "step-1",
		TurnID:         "turn-1",
		TaskID:         "task-1",
		SpiritSessionID: "sess-1",
		Kind:           biz.StepKindReply,
		AuthorAgentKey: "agent-key-1",
		Status:         biz.StepStatusRunning,
		Version:        1,
	}))

	v1.mu.Lock()
	defer v1.mu.Unlock()
	if len(v1.pub) != 1 {
		t.Fatalf("expected 1 v1 event, got %d", len(v1.pub))
	}
	ev := v1.pub[0]
	if ev.Event != biz.ActivityEventCreated {
		t.Errorf("expected v1 event type Created, got %s", ev.Event)
	}
	if ev.Activity.Kind != biz.ActivityKindReply {
		t.Errorf("expected v1 kind reply, got %s", ev.Activity.Kind)
	}
	if ev.Activity.AgentKey != "agent-key-1" {
		t.Errorf("expected AgentKey agent-key-1, got %s", ev.Activity.AgentKey)
	}
}

// TestCompatAdapter_UnknownEventDropped verifies that events with no v1
// mapping (e.g. TurnStartedEvent) are silently dropped.
func TestCompatAdapter_UnknownEventDropped(t *testing.T) {
	t.Parallel()
	v1 := &fakeV1Bus{}
	adapter := NewCompatAdapter(v1)

	adapter.PublishV1(context.Background(), biz.NewTurnStartedEvent(biz.Turn{
		ID:              "turn-1",
		TaskID:          "task-1",
		SpiritSessionID: "sess-1",
		Version:         1,
	}))

	v1.mu.Lock()
	defer v1.mu.Unlock()
	if len(v1.pub) != 0 {
		t.Fatalf("expected 0 v1 events (turn has no mapping), got %d", len(v1.pub))
	}
}
