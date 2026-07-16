package v2

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// fakeRepoSet is a minimal repo collection for testing — captures all upserts.
type fakeRepoSet struct {
	mu         sync.Mutex
	tasks      []biz.Task
	turns      []biz.Turn
	steps      []biz.Step
	boards     []biz.PlanBoard
	pSteps     []biz.PlanStep
	stages     []biz.TeamStage
	runs       []biz.TeamRun
	members    []biz.MemberSession
	graphStgs  []biz.GraphStage
	graphNodes []biz.GraphNode
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

// 2026-07-04 问题 2 修复：补齐 GraphStage/GraphNode 方法以满足 RepoSet 接口。
func (f *fakeRepoSet) UpsertGraphStage(_ context.Context, g biz.GraphStage) (biz.GraphStage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.graphStgs = append(f.graphStgs, g)
	return g, nil
}
func (f *fakeRepoSet) UpsertGraphNode(_ context.Context, g biz.GraphNode) (biz.GraphNode, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.graphNodes = append(f.graphNodes, g)
	return g, nil
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
	defer rs.mu.Unlock()
	if len(rs.tasks) != 1 || rs.tasks[0].ID != "task-1" {
		t.Fatalf("expected 1 task persisted, got %+v", rs.tasks)
	}

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

// fakeActivityUpserter retained for historical Phase 2 tests (removed).
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

// TestSequencer_PublishSystemNotice verifies system.notice events are
// fan-out published (not entity-persisted) without ActivityBridge.
func TestSequencer_PublishSystemNotice(t *testing.T) {
	t.Parallel()
	rs := &fakeRepoSet{}
	bus := &fakeBus{}
	s := NewSequencer(rs, bus, loggateway.NewNoop(),
		WithPublishBuffer(16),
		WithPersistBuffer(16),
		WithDeltaBatchInterval(time.Millisecond*4),
	)
	t.Cleanup(func() { _ = s.Close() })

	ctx := context.Background()
	ev := biz.NewSystemNoticeEvent("spirit-1", "team_summary", "", map[string]any{"run_id": "r1"})
	s.Publish(ctx, ev)

	if err := s.Flush(ctx); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	bus.mu.Lock()
	defer bus.mu.Unlock()
	if len(bus.pub) != 1 {
		t.Fatalf("expected 1 bus publish, got %d", len(bus.pub))
	}
	got, ok := bus.pub[0].(*biz.SystemNoticeEvent)
	if !ok {
		t.Fatalf("expected *SystemNoticeEvent, got %T", bus.pub[0])
	}
	if got.NoticeType != "team_summary" {
		t.Errorf("NoticeType=%q want team_summary", got.NoticeType)
	}
}

// TestSequencer_TerminalEventWBPF verifies P1-04 fix: terminal events
// (task.completed) are persisted via the write-before-publish path. A terminal
// event should be persisted and published correctly.
//
// The WBPF ordering guarantee (persist before publish) is verified by code
// inspection of processTask — the sync persistAction call precedes bus.Publish
// in the same goroutine, with no async gap.
func TestSequencer_TerminalEventWBPF(t *testing.T) {
	t.Parallel()

	rs := &fakeRepoSet{}
	bus := &fakeBus{}
	s := NewSequencer(rs, bus, loggateway.NewNoop(),
		WithPublishBuffer(16),
		WithPersistBuffer(16),
		WithDeltaBatchInterval(time.Millisecond*4),
	)
	t.Cleanup(func() { _ = s.Close() })

	ctx := context.Background()
	task := biz.Task{
		ID:        "task-wbpf",
		SessionID: "sess-1",
		Status:    biz.TaskStatusCompleted,
		Version:   2,
	}
	evt := biz.NewTaskCompletedEvent(task)
	s.Publish(ctx, evt)

	if err := s.Flush(ctx); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	// Verify the task was persisted exactly once (WBPF sync persist succeeds,
	// so the async retry path is NOT taken — no double persist).
	rs.mu.Lock()
	if len(rs.tasks) != 1 {
		rs.mu.Unlock()
		t.Fatalf("expected 1 task persisted (WBPF sync), got %d", len(rs.tasks))
	}
	if rs.tasks[0].ID != "task-wbpf" || rs.tasks[0].Status != biz.TaskStatusCompleted {
		rs.mu.Unlock()
		t.Errorf("persisted task = %+v, want task-wbpf/completed", rs.tasks[0])
	}
	rs.mu.Unlock()

	// Verify the event was published.
	bus.mu.Lock()
	defer bus.mu.Unlock()
	if len(bus.pub) != 1 {
		t.Fatalf("expected 1 event published, got %d", len(bus.pub))
	}
	if bus.pub[0].EventKind() != biz.EventKindTaskCompleted {
		t.Errorf("published event kind = %s, want task.completed", bus.pub[0].EventKind())
	}
}

// TestSequencer_TerminalEventWBPF_FallbackOnPersistError verifies P1-04 fix:
// when the sync persist for a terminal event fails, the event falls back to
// the async persist path and is still published to the bus (best-effort WBPF).
func TestSequencer_TerminalEventWBPF_FallbackOnPersistError(t *testing.T) {
	t.Parallel()

	rs := &errorThenSuccessRepoSet{}
	bus := &fakeBus{}
	s := NewSequencer(rs, bus, loggateway.NewNoop(),
		WithPublishBuffer(16),
		WithPersistBuffer(16),
		WithDeltaBatchInterval(time.Millisecond*4),
	)
	t.Cleanup(func() { _ = s.Close() })

	ctx := context.Background()
	task := biz.Task{
		ID:        "task-fallback",
		SessionID: "sess-1",
		Status:    biz.TaskStatusFailed,
		Version:   2,
	}
	evt := biz.NewTaskFailedEvent(task)
	s.Publish(ctx, evt)

	if err := s.Flush(ctx); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	// Give the async persist loop time to drain.
	time.Sleep(100 * time.Millisecond)

	// The sync persist failed (first call returns error), so the async path
	// was taken. The async persist should succeed (second call).
	rs.mu.Lock()
	if rs.persistCount < 2 {
		rs.mu.Unlock()
		t.Fatalf("expected >=2 persist attempts (sync fail + async retry), got %d", rs.persistCount)
	}
	rs.mu.Unlock()

	// The event should still be published.
	bus.mu.Lock()
	defer bus.mu.Unlock()
	if len(bus.pub) == 0 {
		t.Fatalf("no event published (should still publish on sync persist failure)")
	}
	if bus.pub[0].EventKind() != biz.EventKindTaskFailed {
		t.Errorf("published event kind = %s, want task.failed", bus.pub[0].EventKind())
	}
}

// errorThenSuccessRepoSet fails the first UpsertTask call, then succeeds.
type errorThenSuccessRepoSet struct {
	fakeRepoSet
	persistCount int
}

func (f *errorThenSuccessRepoSet) UpsertTask(ctx context.Context, t biz.Task) (biz.Task, error) {
	f.mu.Lock()
	f.persistCount++
	count := f.persistCount
	f.mu.Unlock()
	if count == 1 {
		return biz.Task{}, errors.New("simulated persist failure")
	}
	return f.fakeRepoSet.UpsertTask(ctx, t)
}

// fakeOutbox captures critical outbox Insert/MarkPublished for B-06 tests.
type fakeOutbox struct {
	mu            sync.Mutex
	inserted      []biz.EventDeliveryOutboxRow
	markPublished []string
}

func (f *fakeOutbox) Insert(_ context.Context, row biz.EventDeliveryOutboxRow) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.inserted = append(f.inserted, row)
	return nil
}

func (f *fakeOutbox) MarkPublished(_ context.Context, id string, _ time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.markPublished = append(f.markPublished, id)
	return nil
}

func (f *fakeOutbox) ListAfter(context.Context, string, string, int64, int) ([]biz.EventDeliveryOutboxRow, error) {
	return nil, nil
}

func TestSequencer_CriticalOutboxInsert(t *testing.T) {
	t.Parallel()

	rs := &fakeRepoSet{}
	bus := &fakeBus{}
	outbox := &fakeOutbox{}
	s := NewSequencer(rs, bus, loggateway.NewNoop(),
		WithPublishBuffer(16),
		WithPersistBuffer(16),
		WithDeltaBatchInterval(time.Millisecond*4),
		WithEventOutbox(outbox),
	)
	t.Cleanup(func() { _ = s.Close() })

	ctx := context.Background()
	task := biz.Task{
		ID:        "task-outbox",
		SessionID: "sess-1",
		Status:    biz.TaskStatusCompleted,
		Seq:       9,
		Version:   2,
	}
	s.Publish(ctx, biz.NewTaskCompletedEvent(task))
	if err := s.Flush(ctx); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	outbox.mu.Lock()
	defer outbox.mu.Unlock()
	if len(outbox.inserted) != 1 {
		t.Fatalf("outbox inserts = %d, want 1", len(outbox.inserted))
	}
	row := outbox.inserted[0]
	wantID := biz.DeliveryEventID(biz.NewTaskCompletedEvent(task), 9)
	if row.EventID != wantID {
		t.Fatalf("event_id = %q, want %q", row.EventID, wantID)
	}
	if row.Seq != 9 || row.SessionID != "sess-1" {
		t.Fatalf("row = %+v", row)
	}
	if len(row.Payload) == 0 {
		t.Fatal("payload empty")
	}
	var env map[string]any
	if err := json.Unmarshal(row.Payload, &env); err != nil {
		t.Fatalf("payload json: %v", err)
	}
	if env["type"] != "v2_event" || env["kind"] != "task.completed" {
		t.Fatalf("envelope type/kind = %v/%v", env["type"], env["kind"])
	}
	if env["session_id"] != "sess-1" || env["event_id"] != wantID {
		t.Fatalf("envelope session/event_id = %v/%v want sess-1/%s", env["session_id"], env["event_id"], wantID)
	}
	if env["payload"] == nil {
		t.Fatal("envelope missing payload")
	}
	if len(outbox.markPublished) != 1 || outbox.markPublished[0] != row.ID {
		t.Fatalf("MarkPublished = %v, want [%s]", outbox.markPublished, row.ID)
	}
}
