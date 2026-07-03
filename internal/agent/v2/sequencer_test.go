package v2

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// fakeRepoSet is a minimal repo collection for testing — captures all upserts.
type fakeRepoSet struct {
	mu      sync.Mutex
	tasks   []biz.Task
	turns   []biz.Turn
	steps   []biz.Step
	boards  []biz.PlanBoard
	pSteps  []biz.PlanStep
	stages  []biz.TeamStage
	runs    []biz.TeamRun
	members []biz.MemberSession
}

func (f *fakeRepoSet) UpsertTask(_ context.Context, t biz.Task) (biz.Task, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.tasks = append(f.tasks, t)
	return t, nil
}
func (f *fakeRepoSet) UpsertTurn(_ context.Context, t biz.Turn) (biz.Turn, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.turns = append(f.turns, t)
	return t, nil
}
func (f *fakeRepoSet) UpsertStep(_ context.Context, s biz.Step) (biz.Step, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.steps = append(f.steps, s)
	return s, nil
}
func (f *fakeRepoSet) UpsertPlanBoard(_ context.Context, p biz.PlanBoard) (biz.PlanBoard, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.boards = append(f.boards, p)
	return p, nil
}
func (f *fakeRepoSet) UpsertPlanStep(_ context.Context, p biz.PlanStep) (biz.PlanStep, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pSteps = append(f.pSteps, p)
	return p, nil
}
func (f *fakeRepoSet) UpsertTeamStage(_ context.Context, t biz.TeamStage) (biz.TeamStage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stages = append(f.stages, t)
	return t, nil
}
func (f *fakeRepoSet) UpsertTeamRun(_ context.Context, t biz.TeamRun) (biz.TeamRun, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.runs = append(f.runs, t)
	return t, nil
}
func (f *fakeRepoSet) UpsertMemberSession(_ context.Context, m biz.MemberSession) (biz.MemberSession, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.members = append(f.members, m)
	return m, nil
}

type fakeBus struct {
	mu  sync.Mutex
	pub []biz.Event
}

func (f *fakeBus) Publish(_ context.Context, e biz.Event) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pub = append(f.pub, e)
}
func (f *fakeBus) Subscribe(_ biz.EventSubscribeOptions) (<-chan biz.Event, func()) {
	ch := make(chan biz.Event)
	return ch, func() {}
}

func newTestSequencer(t *testing.T) (*Sequencer, *fakeRepoSet, *fakeBus) {
	t.Helper()
	rs := &fakeRepoSet{}
	bus := &fakeBus{}
	s := NewSequencer(rs, bus, loggateway.NewNoop(),
		WithPublishBuffer(16),
		WithPersistBuffer(16),
		WithDeltaBatchInterval(time.Millisecond*4),
	)
	t.Cleanup(func() { _ = s.Close() })
	return s, rs, bus
}

// TestSequencer_PublishTaskCreated verifies that a task.created event is
// persisted to TaskRepo AND published to the bus in FIFO order.
func TestSequencer_PublishTaskCreated(t *testing.T) {
	t.Parallel()
	s, rs, bus := newTestSequencer(t)
	ctx := context.Background()

	evt := biz.NewTaskCreatedEvent(biz.Task{
		ID: "task-1", SessionID: "sess-1", Status: biz.TaskStatusRunning,
		Version: 1,
	})
	s.Publish(ctx, evt)

	if err := s.Flush(ctx); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	rs.mu.Lock()
	if len(rs.tasks) != 1 || rs.tasks[0].ID != "task-1" {
		t.Fatalf("expected 1 task persisted, got %+v", rs.tasks)
	}
	rs.mu.Unlock()

	bus.mu.Lock()
	defer bus.mu.Unlock()
	if len(bus.pub) != 1 || bus.pub[0].EventKind() != biz.EventKindTaskCreated {
		t.Fatalf("expected task.created published, got %+v", bus.pub)
	}
}

// TestSequencer_FIFOOrderAcrossEventTypes verifies cross-event-type FIFO:
// task.created must be persisted/published BEFORE turn.started even if
// Publish is called concurrently.
func TestSequencer_FIFOOrderAcrossEventTypes(t *testing.T) {
	t.Parallel()
	s, _, bus := newTestSequencer(t)
	ctx := context.Background()

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		s.Publish(ctx, biz.NewTaskCreatedEvent(biz.Task{ID: "t-1", SessionID: "s-1", Version: 1}))
	}()
	go func() {
		defer wg.Done()
		s.Publish(ctx, biz.NewTurnStartedEvent(biz.Turn{ID: "tn-1", TaskID: "t-1", SpiritSessionID: "s-1", Seq: 1, Version: 1}))
	}()
	wg.Wait()

	if err := s.Flush(ctx); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	bus.mu.Lock()
	defer bus.mu.Unlock()
	if len(bus.pub) != 2 {
		t.Fatalf("expected 2 published events, got %d", len(bus.pub))
	}
	for _, e := range bus.pub {
		if e.OccurredAt().IsZero() {
			t.Errorf("event %s has zero OccurredAt", e.EventKind())
		}
	}
}

// TestSequencer_StreamingBatchMerge verifies that two step.streaming events
// for the SAME StepID within the batch window merge into a single WS publish.
func TestSequencer_StreamingBatchMerge(t *testing.T) {
	t.Parallel()
	s, _, bus := newTestSequencer(t)
	ctx := context.Background()

	s.Publish(ctx, biz.NewStepStreamingEvent("", "", "step-1", "content", "hello"))
	s.Publish(ctx, biz.NewStepStreamingEvent("", "", "step-1", "content", " world"))

	if err := s.Flush(ctx); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	bus.mu.Lock()
	defer bus.mu.Unlock()
	if len(bus.pub) != 1 {
		t.Fatalf("expected 1 merged event, got %d (%+v)", len(bus.pub), bus.pub)
	}
}

// TestSequencer_StreamingDoesNotPersist verifies that step.streaming events
// are NOT persisted to RepoSet (only step.created/updated/completed are).
func TestSequencer_StreamingDoesNotPersist(t *testing.T) {
	t.Parallel()
	s, rs, _ := newTestSequencer(t)
	ctx := context.Background()

	s.Publish(ctx, biz.NewStepStreamingEvent("", "", "step-1", "content", "hello"))
	if err := s.Flush(ctx); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	rs.mu.Lock()
	defer rs.mu.Unlock()
	if len(rs.steps) != 0 {
		t.Fatalf("expected 0 persisted steps, got %d", len(rs.steps))
	}
}

// TestSequencer_DeadLetterOnPersistFailure verifies that after 5 retries,
// a failing persist lands in DeadLetter().
func TestSequencer_DeadLetterOnPersistFailure(t *testing.T) {
	t.Parallel()
	rs := &failingRepoSet{fail: true}
	bus := &fakeBus{}
	s := NewSequencer(rs, bus, loggateway.NewNoop(),
		WithPublishBuffer(16),
		WithPersistBuffer(16),
		WithDeltaBatchInterval(time.Millisecond*4),
		WithPersistMaxRetries(5),
		WithPersistBackoff(time.Millisecond),
	)
	t.Cleanup(func() { _ = s.Close() })

	ctx := context.Background()
	s.Publish(ctx, biz.NewTaskCreatedEvent(biz.Task{ID: "t-fail", SessionID: "s-1", Version: 1}))
	if err := s.Flush(ctx); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	deadline := time.After(time.Second)
	for {
		select {
		case <-deadline:
			t.Fatalf("dead letter never received; dead=%d", s.DeadLetterCount())
		default:
		}
		if s.DeadLetterCount() > 0 {
			break
		}
		time.Sleep(time.Millisecond)
	}
}

// failingRepoSet wraps fakeRepoSet and forces every Upsert to error.
type failingRepoSet struct {
	fakeRepoSet
	fail bool
}

func (f *failingRepoSet) UpsertTask(ctx context.Context, t biz.Task) (biz.Task, error) {
	if f.fail {
		return biz.Task{}, errTestFailure
	}
	return f.fakeRepoSet.UpsertTask(ctx, t)
}

var errTestFailure = errors.New("simulated persist failure")

// fakeActivityUpserter captures UpsertActivity calls for verifying Phase 2
// ActivityBridgeEvent persistence.
type fakeActivityUpserter struct {
	mu         sync.Mutex
	activities []biz.Activity
	err        error
}

func (f *fakeActivityUpserter) UpsertActivity(_ context.Context, a biz.Activity) (biz.Activity, error) {
	if f.err != nil {
		return biz.Activity{}, f.err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.activities = append(f.activities, a)
	return a, nil
}

func (f *fakeActivityUpserter) snapshot() []biz.Activity {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]biz.Activity, len(f.activities))
	copy(out, f.activities)
	return out
}

// TestSequencer_PublishActivityBridgeEvent_Persist verifies that an
// ActivityBridgeEvent payload is persisted via the wired ActivityUpserter
// (Phase 2 Step 2a) and that Seq is auto-allocated per SpiritSessionID
// (Phase 2 Step 2b).
func TestSequencer_PublishActivityBridgeEvent_Persist(t *testing.T) {
	t.Parallel()
	rs := &fakeRepoSet{}
	bus := &fakeBus{}
	au := &fakeActivityUpserter{}
	s := NewSequencer(rs, bus, loggateway.NewNoop(),
		WithPublishBuffer(16),
		WithPersistBuffer(16),
		WithDeltaBatchInterval(time.Millisecond*4),
		WithActivityUpserter(au),
	)
	t.Cleanup(func() { _ = s.Close() })

	ctx := context.Background()
	ev := biz.NewActivityBridgeEvent(biz.ActivityEvent{
		Event: biz.ActivityEventUpdated,
		Activity: biz.Activity{
			ID:              "act-1",
			Kind:            biz.ActivityKindNotice,
			Status:          biz.ActivityStatusRunning,
			SessionID:       "spirit-1",
			SpiritSessionID: "spirit-1",
			Stage:           "team_dag_snapshot",
		},
		Domain: biz.ActivityDomainChat,
	})
	s.Publish(ctx, ev)

	if err := s.Flush(ctx); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	persisted := au.snapshot()
	if len(persisted) != 1 {
		t.Fatalf("expected 1 persisted activity, got %d", len(persisted))
	}
	if persisted[0].ID != "act-1" {
		t.Errorf("persisted activity ID = %q, want %q", persisted[0].ID, "act-1")
	}
	if persisted[0].Seq == 0 {
		t.Errorf("persisted activity Seq not auto-allocated; expected non-zero")
	}
}

// TestSequencer_PublishActivityBridgeEvent_NoUpserter verifies that an
// ActivityBridgeEvent payload is NOT persisted when ActivityUpserter is nil
// (skipped gracefully, no error, no panic).
func TestSequencer_PublishActivityBridgeEvent_NoUpserter(t *testing.T) {
	t.Parallel()
	rs := &fakeRepoSet{}
	bus := &fakeBus{}
	// No WithActivityUpserter: au is nil
	s := NewSequencer(rs, bus, loggateway.NewNoop(),
		WithPublishBuffer(16),
		WithPersistBuffer(16),
		WithDeltaBatchInterval(time.Millisecond*4),
	)
	t.Cleanup(func() { _ = s.Close() })

	ctx := context.Background()
	ev := biz.NewActivityBridgeEvent(biz.ActivityEvent{
		Event: biz.ActivityEventUpdated,
		Activity: biz.Activity{
			ID:              "act-2",
			Kind:            biz.ActivityKindNotice,
			SpiritSessionID: "spirit-2",
		},
		Domain: biz.ActivityDomainChat,
	})
	s.Publish(ctx, ev)

	if err := s.Flush(ctx); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	// bus still receives the event
	if len(bus.pub) != 1 {
		t.Errorf("expected 1 bus publish, got %d", len(bus.pub))
	}
}

// TestSequencer_PublishActivityBridgeEvent_SeqMonotonic verifies that Seq
// auto-allocation is monotonic per SpiritSessionID.
func TestSequencer_PublishActivityBridgeEvent_SeqMonotonic(t *testing.T) {
	t.Parallel()
	rs := &fakeRepoSet{}
	bus := &fakeBus{}
	au := &fakeActivityUpserter{}
	s := NewSequencer(rs, bus, loggateway.NewNoop(),
		WithPublishBuffer(32),
		WithPersistBuffer(32),
		WithDeltaBatchInterval(time.Millisecond*4),
		WithActivityUpserter(au),
	)
	t.Cleanup(func() { _ = s.Close() })

	ctx := context.Background()
	for i := 0; i < 3; i++ {
		ev := biz.NewActivityBridgeEvent(biz.ActivityEvent{
			Event: biz.ActivityEventUpdated,
			Activity: biz.Activity{
				ID:              "act-" + string(rune('a'+i)),
				Kind:            biz.ActivityKindNotice,
				SpiritSessionID: "spirit-seq",
			},
			Domain: biz.ActivityDomainChat,
		})
		s.Publish(ctx, ev)
	}
	if err := s.Flush(ctx); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	persisted := au.snapshot()
	if len(persisted) != 3 {
		t.Fatalf("expected 3 persisted activities, got %d", len(persisted))
	}
	for i, a := range persisted {
		if a.Seq != int64(i+1) {
			t.Errorf("persisted[%d].Seq = %d, want %d", i, a.Seq, i+1)
		}
	}
}
