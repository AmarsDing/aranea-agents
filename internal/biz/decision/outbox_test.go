package decision

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"aranea-agents/internal/workspace"
)

// fakeRepo records calls for worker behavior assertions.
type fakeRepo struct {
	mu sync.Mutex

	inserted    []Record
	insertErrs  []error // consumed one per InsertRecords call
	insertCalls int

	enqueued     []Record
	enqueueErr   error
	enqueueCalls int

	pending      []OutboxRow
	listErr      error
	publishedIDs []int64
	deadIDs      []int64
	attempts     map[int64]string
}

func (f *fakeRepo) InsertRecords(_ context.Context, recs []Record) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.insertCalls++
	if len(f.insertErrs) > 0 {
		err := f.insertErrs[0]
		f.insertErrs = f.insertErrs[1:]
		if err != nil {
			return err
		}
	}
	f.inserted = append(f.inserted, recs...)
	return nil
}

func (f *fakeRepo) EnqueueOutbox(_ context.Context, recs []Record) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.enqueueCalls++
	if f.enqueueErr != nil {
		return f.enqueueErr
	}
	f.enqueued = append(f.enqueued, recs...)
	return nil
}

func (f *fakeRepo) ListPendingOutbox(_ context.Context, _ int) ([]OutboxRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.listErr != nil {
		return nil, f.listErr
	}
	out := make([]OutboxRow, len(f.pending))
	copy(out, f.pending)
	return out, nil
}

func (f *fakeRepo) MarkOutboxPublished(_ context.Context, ids []int64, _ time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.publishedIDs = append(f.publishedIDs, ids...)
	return nil
}

func (f *fakeRepo) MarkOutboxAttempt(_ context.Context, id int64, lastError string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.attempts == nil {
		f.attempts = map[int64]string{}
	}
	f.attempts[id] = lastError
	return nil
}

func (f *fakeRepo) MarkOutboxDead(_ context.Context, ids []int64, _ string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deadIDs = append(f.deadIDs, ids...)
	return nil
}

func (f *fakeRepo) snapshot() (inserted, enqueued []Record, insertCalls, enqueueCalls int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]Record(nil), f.inserted...), append([]Record(nil), f.enqueued...), f.insertCalls, f.enqueueCalls
}

func TestNoopCollector(t *testing.T) {
	NewNoopCollector().Emit(context.Background(), Record{}) // must not panic
}

// waitFor polls cond until true or the deadline passes.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestOutboxCollector_EmitValidates(t *testing.T) {
	repo := &fakeRepo{}
	c := NewOutboxCollector(repo, nil, WithFlushInterval(time.Hour)) // never auto-flush
	oc := asOutbox(t, c)
	// Worker not started: channel state is deterministic.

	oc.Emit(context.Background(), Record{}) // invalid: dropped
	oc.Emit(context.Background(), validRecord())

	if len(oc.ch) != 1 {
		t.Fatalf("channel len = %d, want 1 (invalid dropped)", len(oc.ch))
	}
	inserted, _, _, _ := repo.snapshot()
	if len(inserted) != 0 {
		t.Fatalf("nothing should be flushed, got %d", len(inserted))
	}
}

func TestOutboxCollector_EmitNonBlockingWhenFull(t *testing.T) {
	repo := &fakeRepo{}
	c := NewOutboxCollector(repo, nil, WithChannelCapacity(2), WithFlushInterval(time.Hour))
	oc := asOutbox(t, c)
	// Do NOT start the worker: channel stays full.
	for i := 0; i < 10; i++ {
		done := make(chan struct{})
		go func() {
			oc.Emit(context.Background(), validRecord())
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(200 * time.Millisecond):
			t.Fatalf("Emit blocked on full channel (NFR-80-01 violation)")
		}
	}
	if len(oc.ch) != 2 {
		t.Fatalf("channel len = %d, want 2 (rest dropped)", len(oc.ch))
	}
}

func TestOutboxCollector_BatchFlushBySize(t *testing.T) {
	repo := &fakeRepo{}
	c := NewOutboxCollector(repo, nil, WithBatchSize(3), WithFlushInterval(time.Hour))
	oc := asOutbox(t, c)
	oc.Start(context.Background())
	defer oc.Stop()

	for i := 0; i < 3; i++ {
		oc.Emit(context.Background(), validRecord())
	}
	waitFor(t, "size-triggered flush", func() bool {
		inserted, _, _, _ := repo.snapshot()
		return len(inserted) == 3
	})
}

func TestOutboxCollector_BatchFlushByTimer(t *testing.T) {
	repo := &fakeRepo{}
	c := NewOutboxCollector(repo, nil, WithBatchSize(100), WithFlushInterval(20*time.Millisecond))
	oc := asOutbox(t, c)
	oc.Start(context.Background())
	defer oc.Stop()

	oc.Emit(context.Background(), validRecord())
	waitFor(t, "timer-triggered flush", func() bool {
		inserted, _, _, _ := repo.snapshot()
		return len(inserted) == 1
	})
}

func TestOutboxCollector_FlushFailureRetriesThenEnqueues(t *testing.T) {
	repo := &fakeRepo{
		insertErrs: []error{errors.New("db down"), errors.New("db down"), errors.New("db down")},
	}
	c := NewOutboxCollector(repo, nil, WithBatchSize(1), WithFlushInterval(time.Hour))
	oc := asOutbox(t, c)
	oc.Start(context.Background())
	defer oc.Stop()

	oc.Emit(context.Background(), validRecord())
	waitFor(t, "retry-queue enqueue after 3 failed flushes", func() bool {
		_, enqueued, insertCalls, enqueueCalls := repo.snapshot()
		return insertCalls == maxFlushAttempts && enqueueCalls == 1 && len(enqueued) == 1
	})
}

func TestOutboxCollector_FlushSucceedsOnRetry(t *testing.T) {
	repo := &fakeRepo{insertErrs: []error{errors.New("flaky")}}
	c := NewOutboxCollector(repo, nil, WithBatchSize(1), WithFlushInterval(time.Hour))
	oc := asOutbox(t, c)
	oc.Start(context.Background())
	defer oc.Stop()

	oc.Emit(context.Background(), validRecord())
	waitFor(t, "second attempt succeeds", func() bool {
		inserted, enqueued, insertCalls, _ := repo.snapshot()
		return len(inserted) == 1 && insertCalls == 2 && len(enqueued) == 0
	})
}

func TestOutboxCollector_ReplayPendingOnStart(t *testing.T) {
	rec := validRecord()
	raw, err := encodeRecord(rec)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	repo := &fakeRepo{pending: []OutboxRow{
		{ID: 7, DecisionKey: rec.DecisionKey, Payload: raw, Status: OutboxStatusPending},
		{ID: 8, DecisionKey: "poison", Payload: []byte("{bad"), Status: OutboxStatusPending},
	}}
	c := NewOutboxCollector(repo, nil, WithFlushInterval(time.Hour))
	oc := asOutbox(t, c)
	oc.Start(context.Background())
	defer oc.Stop()

	waitFor(t, "startup replay", func() bool {
		inserted, _, _, _ := repo.snapshot()
		repo.mu.Lock()
		defer repo.mu.Unlock()
		// 7 replayed→published；8 poison→dead（t-dr-4：不可解码行重试永不
		// 成功，标 published 是审计造假——记录从未投递）。
		return len(inserted) == 1 && len(repo.publishedIDs) == 1 && len(repo.deadIDs) == 1 &&
			repo.publishedIDs[0] == 7 && repo.deadIDs[0] == 8
	})
}

func TestOutboxCollector_ReplayFailureMarksAttempt(t *testing.T) {
	rec := validRecord()
	raw, err := encodeRecord(rec)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	repo := &fakeRepo{
		pending:    []OutboxRow{{ID: 9, DecisionKey: rec.DecisionKey, Payload: raw, Status: OutboxStatusPending}},
		insertErrs: []error{errors.New("still down")},
	}
	c := NewOutboxCollector(repo, nil, WithFlushInterval(time.Hour))
	oc := asOutbox(t, c)
	oc.Start(context.Background())
	defer oc.Stop()

	waitFor(t, "attempt marked", func() bool {
		repo.mu.Lock()
		defer repo.mu.Unlock()
		_, marked := repo.attempts[9]
		return marked && len(repo.publishedIDs) == 0
	})
}

func TestOutboxCollector_StopDrains(t *testing.T) {
	repo := &fakeRepo{}
	c := NewOutboxCollector(repo, nil, WithBatchSize(100), WithFlushInterval(time.Hour))
	oc := asOutbox(t, c)
	oc.Start(context.Background())

	for i := 0; i < 5; i++ {
		oc.Emit(context.Background(), validRecord())
	}
	oc.Stop() // must flush remaining buffer before returning

	inserted, _, _, _ := repo.snapshot()
	if len(inserted) != 5 {
		t.Fatalf("stop-drain inserted = %d, want 5", len(inserted))
	}
}

// asOutbox unwraps the Collector interface to the concrete type for tests.
func asOutbox(t *testing.T, c Collector) *outboxCollector {
	t.Helper()
	oc, ok := c.(*outboxCollector)
	if !ok {
		t.Fatalf("NewOutboxCollector returned %T, want *outboxCollector", c)
	}
	return oc
}

// TestOutboxCollector_EmitBackfillsWorkspace pins t-dr-2：emit 方不显式填
// workspace_id 时，收口点按 caller workspace 声明回填；无声明路径落
// DefaultWorkspaceID；显式已填不被覆盖（emit 方优先）。
func TestOutboxCollector_EmitBackfillsWorkspace(t *testing.T) {
	repo := &fakeRepo{}
	c := NewOutboxCollector(repo, nil, WithFlushInterval(time.Hour))
	oc := asOutbox(t, c)

	// ① ctx 带租户声明 → 回填该租户。
	oc.Emit(workspace.WithContext(context.Background(), "ws-tenant"), validRecord())
	// ② ctx 无声明 → DefaultWorkspaceID（与团队/代理默认工作区口径一致）。
	oc.Emit(context.Background(), validRecord())
	// ③ emit 方显式已填 → 不覆盖。
	explicit := validRecord()
	explicit.WorkspaceID = "ws-explicit"
	oc.Emit(workspace.WithContext(context.Background(), "ws-tenant"), explicit)

	if len(oc.ch) != 3 {
		t.Fatalf("channel len = %d, want 3", len(oc.ch))
	}
	got := make([]string, 0, 3)
	for i := 0; i < 3; i++ {
		got = append(got, (<-oc.ch).WorkspaceID)
	}
	want := []string{"ws-tenant", workspace.DefaultWorkspaceID, "ws-explicit"}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("rec[%d].WorkspaceID = %q, want %q", i, got[i], want[i])
		}
	}
}
