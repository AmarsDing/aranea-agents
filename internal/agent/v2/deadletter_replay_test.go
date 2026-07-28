package v2

// deadletter_replay_test.go — P1-R2b: durable dead-letter save + replay tests.

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// fakeDeadLetterStore is an in-memory biz.EventDeadLetterRepo.
type fakeDeadLetterStore struct {
	mu        sync.Mutex
	saved     []biz.EventDeadLetter
	pending   []biz.EventDeadLetter
	replayed  []int64
	abandoned map[int64]string
	attempts  map[int64]int
}

func newFakeDeadLetterStore() *fakeDeadLetterStore {
	return &fakeDeadLetterStore{
		abandoned: map[int64]string{},
		attempts:  map[int64]int{},
	}
}

func (f *fakeDeadLetterStore) SaveEventDeadLetter(_ context.Context, rec biz.EventDeadLetter) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.saved = append(f.saved, rec)
	return nil
}

func (f *fakeDeadLetterStore) ListPendingEventDeadLetters(_ context.Context, limit int) ([]biz.EventDeadLetter, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if limit > 0 && len(f.pending) > limit {
		return f.pending[:limit], nil
	}
	return f.pending, nil
}

func (f *fakeDeadLetterStore) MarkEventDeadLetterReplayed(_ context.Context, id int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.replayed = append(f.replayed, id)
	return nil
}

func (f *fakeDeadLetterStore) MarkEventDeadLetterAbandoned(_ context.Context, id int64, reason string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.abandoned[id] = reason
	return nil
}

func (f *fakeDeadLetterStore) IncrementEventDeadLetterAttempt(_ context.Context, id int64, _ string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.attempts[id]++
	return nil
}

// TestSequencer_DeadLetterSavedToDurableStore verifies that a permanently
// failing persist lands BOTH in the in-memory ring and the durable store,
// with the entity-level routing fields (kind/op/id/payload) needed for replay.
func TestSequencer_DeadLetterSavedToDurableStore(t *testing.T) {
	t.Parallel()
	rs := &failingRepoSet{fail: true}
	store := newFakeDeadLetterStore()
	s := NewSequencer(rs, &fakeBus{}, loggateway.NewNoop(),
		WithPublishBuffer(16),
		WithPersistBuffer(16),
		WithDeltaBatchInterval(time.Millisecond*4),
		WithPersistMaxRetries(2),
		WithPersistBackoff(time.Millisecond),
		WithDeadLetterStore(store),
	)
	t.Cleanup(func() { _ = s.Close() })

	ctx := context.Background()
	s.Publish(ctx, biz.NewTaskCreatedEvent(biz.Task{ID: "t-dl", SessionID: "s-1", Version: 1}))
	if err := s.Flush(ctx); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.saved) != 1 {
		t.Fatalf("expected 1 durable dead-letter, got %d", len(store.saved))
	}
	rec := store.saved[0]
	if rec.EntityKind != EntityKindTask || rec.EntityOp != PersistOpUpsert {
		t.Fatalf("unexpected routing: kind=%s op=%s", rec.EntityKind, rec.EntityOp)
	}
	if rec.EntityID != "t-dl" {
		t.Fatalf("EntityID = %q, want t-dl", rec.EntityID)
	}
	if rec.State != biz.EventDeadLetterStatePending {
		t.Fatalf("State = %q, want pending", rec.State)
	}
	var decoded biz.Task
	if err := json.Unmarshal([]byte(rec.PayloadJSON), &decoded); err != nil {
		t.Fatalf("payload not a biz.Task: %v", err)
	}
	if decoded.ID != "t-dl" || decoded.SessionID != "s-1" {
		t.Fatalf("payload round-trip mismatch: %+v", decoded)
	}
}

// TestSequencer_ReplayDeadLettersOnce_Success verifies a pending record is
// re-applied to the RepoSet and marked replayed.
func TestSequencer_ReplayDeadLettersOnce_Success(t *testing.T) {
	t.Parallel()
	rs := &fakeRepoSet{}
	store := newFakeDeadLetterStore()
	payload, err := json.Marshal(biz.Task{ID: "t-r1", SessionID: "s-1", Status: biz.TaskStatusRunning, Version: 3})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	store.pending = []biz.EventDeadLetter{{
		ID:          7,
		EventKind:   string(biz.EventKindTaskCreated),
		EntityKind:  EntityKindTask,
		EntityOp:    PersistOpUpsert,
		EntityID:    "t-r1",
		SessionID:   "s-1",
		PayloadJSON: string(payload),
		State:       biz.EventDeadLetterStatePending,
	}}

	s := NewSequencer(rs, &fakeBus{}, loggateway.NewNoop(),
		WithPublishBuffer(16), WithPersistBuffer(16),
		WithDeadLetterStore(store),
		// 后台 replay worker 会在启动时立即 sweep 一次，与下方手动调用
		// replayDeadLettersOnce 竞争导致重复应用，测试中必须禁用。
		WithDeadLetterReplayLoopDisabled(),
	)
	t.Cleanup(func() { _ = s.Close() })

	s.replayDeadLettersOnce()

	rs.mu.Lock()
	if len(rs.tasks) != 1 || rs.tasks[0].ID != "t-r1" {
		t.Fatalf("expected task replayed into repo, got %+v", rs.tasks)
	}
	rs.mu.Unlock()

	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.replayed) != 1 || store.replayed[0] != 7 {
		t.Fatalf("expected record 7 marked replayed, got %+v", store.replayed)
	}
	if len(store.abandoned) != 0 {
		t.Fatalf("unexpected abandons: %+v", store.abandoned)
	}
}

// TestSequencer_ReplayDeadLettersOnce_TerminalOp verifies the terminal op
// routes to CompleteTaskTerminal (not UpsertTask) during replay.
func TestSequencer_ReplayDeadLettersOnce_TerminalOp(t *testing.T) {
	t.Parallel()
	rs := &fakeRepoSet{}
	store := newFakeDeadLetterStore()
	payload, _ := json.Marshal(biz.Task{ID: "t-term", SessionID: "s-1", Status: biz.TaskStatusCompleted})
	store.pending = []biz.EventDeadLetter{{
		ID: 9, EntityKind: EntityKindTask, EntityOp: PersistOpCompleteTaskTerminal,
		EntityID: "t-term", PayloadJSON: string(payload), State: biz.EventDeadLetterStatePending,
	}}

	s := NewSequencer(rs, &fakeBus{}, loggateway.NewNoop(),
		WithPublishBuffer(16), WithPersistBuffer(16),
		WithDeadLetterStore(store),
		// 后台 replay worker 会在启动时立即 sweep 一次，与下方手动调用
		// replayDeadLettersOnce 竞争导致重复应用，测试中必须禁用。
		WithDeadLetterReplayLoopDisabled(),
	)
	t.Cleanup(func() { _ = s.Close() })

	s.replayDeadLettersOnce()

	rs.mu.Lock()
	defer rs.mu.Unlock()
	if len(rs.terminal) != 1 || rs.terminal[0].ID != "t-term" {
		t.Fatalf("expected CompleteTaskTerminal called, got %+v", rs.terminal)
	}
	if len(rs.tasks) != 0 {
		t.Fatalf("UpsertTask must not be called for terminal op, got %+v", rs.tasks)
	}
}

// TestSequencer_ReplayDeadLettersOnce_FailureIncrementsAttempt verifies a
// failing replay increments attempts and leaves the record pending.
func TestSequencer_ReplayDeadLettersOnce_FailureIncrementsAttempt(t *testing.T) {
	t.Parallel()
	rs := &failingRepoSet{fail: true}
	store := newFakeDeadLetterStore()
	payload, _ := json.Marshal(biz.Task{ID: "t-f1", SessionID: "s-1", Version: 1})
	store.pending = []biz.EventDeadLetter{{
		ID: 11, EntityKind: EntityKindTask, EntityOp: PersistOpUpsert,
		EntityID: "t-f1", PayloadJSON: string(payload), State: biz.EventDeadLetterStatePending,
	}}

	s := NewSequencer(rs, &fakeBus{}, loggateway.NewNoop(),
		WithPublishBuffer(16), WithPersistBuffer(16),
		WithDeadLetterStore(store),
		// 后台 replay worker 会在启动时立即 sweep 一次，与下方手动调用
		// replayDeadLettersOnce 竞争导致重复应用，测试中必须禁用。
		WithDeadLetterReplayLoopDisabled(),
	)
	t.Cleanup(func() { _ = s.Close() })

	s.replayDeadLettersOnce()

	store.mu.Lock()
	defer store.mu.Unlock()
	if store.attempts[11] != 1 {
		t.Fatalf("attempts = %d, want 1", store.attempts[11])
	}
	if len(store.replayed) != 0 || len(store.abandoned) != 0 {
		t.Fatalf("record must stay pending: replayed=%v abandoned=%v", store.replayed, store.abandoned)
	}
}

// TestSequencer_ReplayDeadLettersOnce_AbandonsAtAttemptCap verifies records at
// the attempt cap are abandoned without calling the repo again.
func TestSequencer_ReplayDeadLettersOnce_AbandonsAtAttemptCap(t *testing.T) {
	t.Parallel()
	rs := &fakeRepoSet{}
	store := newFakeDeadLetterStore()
	payload, _ := json.Marshal(biz.Task{ID: "t-cap", SessionID: "s-1", Version: 1})
	store.pending = []biz.EventDeadLetter{{
		ID: 13, EntityKind: EntityKindTask, EntityOp: PersistOpUpsert,
		EntityID: "t-cap", PayloadJSON: string(payload),
		Attempts: deadLetterMaxReplayAttempt, State: biz.EventDeadLetterStatePending,
	}}

	s := NewSequencer(rs, &fakeBus{}, loggateway.NewNoop(),
		WithPublishBuffer(16), WithPersistBuffer(16),
		WithDeadLetterStore(store),
		// 后台 replay worker 会在启动时立即 sweep 一次，与下方手动调用
		// replayDeadLettersOnce 竞争导致重复应用，测试中必须禁用。
		WithDeadLetterReplayLoopDisabled(),
	)
	t.Cleanup(func() { _ = s.Close() })

	s.replayDeadLettersOnce()

	rs.mu.Lock()
	if len(rs.tasks) != 0 {
		t.Fatalf("repo must not be called at attempt cap, got %+v", rs.tasks)
	}
	rs.mu.Unlock()

	store.mu.Lock()
	defer store.mu.Unlock()
	if store.abandoned[13] != "max_attempts_exceeded" {
		t.Fatalf("abandoned[13] = %q, want max_attempts_exceeded", store.abandoned[13])
	}
}

// TestSequencer_ReplayDeadLettersOnce_AbandonsUndecodablePayload verifies a
// poisoned payload is abandoned immediately (retrying can never succeed).
func TestSequencer_ReplayDeadLettersOnce_AbandonsUndecodablePayload(t *testing.T) {
	t.Parallel()
	rs := &fakeRepoSet{}
	store := newFakeDeadLetterStore()
	store.pending = []biz.EventDeadLetter{{
		ID: 17, EntityKind: EntityKindTask, EntityOp: PersistOpUpsert,
		EntityID: "t-bad", PayloadJSON: "{not-json", State: biz.EventDeadLetterStatePending,
	}}

	s := NewSequencer(rs, &fakeBus{}, loggateway.NewNoop(),
		WithPublishBuffer(16), WithPersistBuffer(16),
		WithDeadLetterStore(store),
		// 后台 replay worker 会在启动时立即 sweep 一次，与下方手动调用
		// replayDeadLettersOnce 竞争导致重复应用，测试中必须禁用。
		WithDeadLetterReplayLoopDisabled(),
	)
	t.Cleanup(func() { _ = s.Close() })

	s.replayDeadLettersOnce()

	store.mu.Lock()
	defer store.mu.Unlock()
	reason, ok := store.abandoned[17]
	if !ok {
		t.Fatalf("expected record 17 abandoned, got %+v", store.abandoned)
	}
	if reason == "" || reason == "max_attempts_exceeded" {
		t.Fatalf("unexpected abandon reason: %q", reason)
	}
}

// TestDescribePersist_StreamingNotPersisted guards the nil-descriptor contract
// for ephemeral events (no dead-letter rows for streaming chunks).
func TestDescribePersist_StreamingNotPersisted(t *testing.T) {
	t.Parallel()
	if d := describePersist(biz.NewStepStreamingEvent("", "", "step-1", "content", "x")); d != nil {
		t.Fatalf("streaming event must not have a persist descriptor, got %+v", d)
	}
}

// TestDecodePersistEntity_RoundTrip covers every entity kind: describe →
// marshal → decode → apply lands in the right repo bucket.
func TestDecodePersistEntity_RoundTrip(t *testing.T) {
	t.Parallel()
	events := []struct {
		name string
		evt  biz.Event
		want string
	}{
		{"task", biz.NewTaskCreatedEvent(biz.Task{ID: "x-task", SessionID: "s", Version: 1}), EntityKindTask},
		{"turn", biz.NewTurnStartedEvent(biz.Turn{ID: "x-turn", TaskID: "x-task", SpiritSessionID: "s", Version: 1}), EntityKindTurn},
		{"step", biz.NewStepCreatedEvent(biz.Step{ID: "x-step", TurnID: "x-turn", TaskID: "x-task", SpiritSessionID: "s", Version: 1}), EntityKindStep},
	}
	for _, tc := range events {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			d := describePersist(tc.evt)
			if d == nil {
				t.Fatalf("describePersist(%s) = nil", tc.name)
			}
			if d.entityKind != tc.want {
				t.Fatalf("entityKind = %q, want %q", d.entityKind, tc.want)
			}
			payload, err := json.Marshal(d.entity)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			entity, err := decodePersistEntity(d.entityKind, payload)
			if err != nil {
				t.Fatalf("decode: %v", err)
			}
			rs := &fakeRepoSet{}
			if err := applyPersist(context.Background(), rs, d.entityKind, d.op, entity); err != nil {
				t.Fatalf("applyPersist: %v", err)
			}
		})
	}
}

// TestApplyPersist_UnknownKind verifies the router rejects unknown kinds
// instead of silently dropping (replay diagnostics rely on the error).
func TestApplyPersist_UnknownKind(t *testing.T) {
	t.Parallel()
	err := applyPersist(context.Background(), &fakeRepoSet{}, "nope", PersistOpUpsert, biz.Task{})
	if err == nil {
		t.Fatal("expected error for unknown entity kind")
	}
	if _, dErr := decodePersistEntity("nope", []byte(`{}`)); dErr == nil {
		t.Fatal("expected decode error for unknown entity kind")
	}
}
