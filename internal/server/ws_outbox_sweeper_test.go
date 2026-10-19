package server

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// fakePendingOutbox implements biz.EventDeliveryOutboxRepo for sweeper tests:
// ListPending serves a fixed batch; MarkPublishedBatch records (and may fail).
type fakePendingOutbox struct {
	mu           sync.Mutex
	pending      []biz.EventDeliveryOutboxRow
	listErr      error
	markErr      error
	markCalls    []string // flattened, in batch order
	markBatches  [][]string
	pendingSeen  []time.Time
	onListCalled func()
}

func (f *fakePendingOutbox) Insert(context.Context, biz.EventDeliveryOutboxRow) error { return nil }

func (f *fakePendingOutbox) MarkPublished(_ context.Context, id string, _ time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.markCalls = append(f.markCalls, id)
	return f.markErr
}

func (f *fakePendingOutbox) MarkPublishedBatch(_ context.Context, ids []string, _ time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.markBatches = append(f.markBatches, append([]string(nil), ids...))
	f.markCalls = append(f.markCalls, ids...)
	return f.markErr
}

func (f *fakePendingOutbox) ListAfter(context.Context, string, string, int64, int) ([]biz.EventDeliveryOutboxRow, error) {
	return nil, nil
}

func (f *fakePendingOutbox) ListPending(_ context.Context, cutoff time.Time, _ int) ([]biz.EventDeliveryOutboxRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pendingSeen = append(f.pendingSeen, cutoff)
	if f.onListCalled != nil {
		f.onListCalled()
	}
	if f.listErr != nil {
		return nil, f.listErr
	}
	return append([]biz.EventDeliveryOutboxRow(nil), f.pending...), nil
}

func TestEventOutboxSweeper_SweepOnceBroadcastsAndMarks(t *testing.T) {
	t.Parallel()
	repo := &fakePendingOutbox{pending: []biz.EventDeliveryOutboxRow{
		{ID: "r1", SessionID: "sess-1", Seq: 7, EventID: "evt-7", Payload: []byte(`{"event_id":"evt-7"}`)},
		{ID: "r2", SessionID: "sess-2", Seq: 3, EventID: "evt-3", Payload: []byte(`{"event_id":"evt-3"}`)},
	}}
	type broadcast struct{ session, payload string }
	var broadcasts []broadcast
	s := newEventOutboxSweeper(repo, func(sessionID string, msg []byte) {
		broadcasts = append(broadcasts, broadcast{sessionID, string(msg)})
	}, loggateway.NewNoop())

	s.sweepOnce(context.Background())

	if len(broadcasts) != 2 {
		t.Fatalf("broadcasts = %v, want 2", broadcasts)
	}
	if broadcasts[0].session != "sess-1" || broadcasts[0].payload != `{"event_id":"evt-7"}` {
		t.Fatalf("first broadcast = %+v", broadcasts[0])
	}
	if broadcasts[1].session != "sess-2" {
		t.Fatalf("second broadcast = %+v", broadcasts[1])
	}
	if len(repo.markCalls) != 2 || repo.markCalls[0] != "r1" || repo.markCalls[1] != "r2" {
		t.Fatalf("markCalls = %v, want [r1 r2]", repo.markCalls)
	}
	// Marks are batched into ONE UPDATE (finding #30): exactly one batch call.
	if len(repo.markBatches) != 1 {
		t.Fatalf("MarkPublishedBatch calls = %d, want 1 (single batch per sweep)", len(repo.markBatches))
	}
	// Cutoff must sit ~grace in the past.
	if len(repo.pendingSeen) != 1 {
		t.Fatalf("ListPending calls = %d", len(repo.pendingSeen))
	}
	if age := time.Since(repo.pendingSeen[0]); age < defaultOutboxSweepGrace || age > defaultOutboxSweepGrace+5*time.Second {
		t.Fatalf("cutoff age = %v, want within [%v, %v]", age, defaultOutboxSweepGrace, defaultOutboxSweepGrace+5*time.Second)
	}
}

func TestEventOutboxSweeper_SweepOnceSkipsUndeliverableButMarks(t *testing.T) {
	t.Parallel()
	repo := &fakePendingOutbox{pending: []biz.EventDeliveryOutboxRow{
		{ID: "empty-payload", SessionID: "sess-1", EventID: "evt-e"},
		{ID: "empty-session", SessionID: "", EventID: "evt-s", Payload: []byte(`{}`)},
		{ID: "ok", SessionID: "sess-9", EventID: "evt-ok", Payload: []byte(`{"a":1}`)},
	}}
	var broadcastCount int
	s := newEventOutboxSweeper(repo, func(string, []byte) { broadcastCount++ }, loggateway.NewNoop())

	s.sweepOnce(context.Background())

	if broadcastCount != 1 {
		t.Fatalf("broadcastCount = %d, want 1 (only deliverable row)", broadcastCount)
	}
	if len(repo.markCalls) != 3 {
		t.Fatalf("markCalls = %v, want all 3 marked (undeliverable rows must not block the window)", repo.markCalls)
	}
}

func TestEventOutboxSweeper_SweepOnceContinuesAfterMarkFailure(t *testing.T) {
	t.Parallel()
	repo := &fakePendingOutbox{
		pending: []biz.EventDeliveryOutboxRow{
			{ID: "fail", SessionID: "sess-1", EventID: "evt-f", Payload: []byte(`{}`)},
			{ID: "ok", SessionID: "sess-1", EventID: "evt-o", Payload: []byte(`{}`)},
		},
		markErr: errors.New("db gone"),
	}
	var broadcastCount int
	s := newEventOutboxSweeper(repo, func(string, []byte) { broadcastCount++ }, loggateway.NewNoop())

	s.sweepOnce(context.Background())

	if broadcastCount != 2 {
		t.Fatalf("broadcastCount = %d, want 2 (mark failure must not stop broadcasting)", broadcastCount)
	}
	// The batch mark is atomic: on failure both rows stay pending and are
	// re-broadcast next tick (at-least-once, clients dedupe by event_id).
	if len(repo.markBatches) != 1 || len(repo.markBatches[0]) != 2 {
		t.Fatalf("markBatches = %v, want one batch of both rows", repo.markBatches)
	}
}

// Budget-exhaustion contract (finding #6/#24): when the shared query/broadcast
// budget burns out mid-batch, the sweep must stop broadcasting the tail —
// those rows are neither re-broadcast this tick nor marked; already-broadcast
// rows still get batch-marked on the marking budget's own timeout.
func TestEventOutboxSweeper_SweepOnceStopsWhenBudgetExhausted(t *testing.T) {
	t.Parallel()
	repo := &fakePendingOutbox{pending: []biz.EventDeliveryOutboxRow{
		{ID: "r1", SessionID: "sess-1", EventID: "evt-1", Payload: []byte(`{}`)},
		{ID: "r2", SessionID: "sess-2", EventID: "evt-2", Payload: []byte(`{}`)},
		{ID: "r3", SessionID: "sess-3", EventID: "evt-3", Payload: []byte(`{}`)},
	}}
	var broadcastCount int
	s := newEventOutboxSweeper(repo, func(string, []byte) {
		broadcastCount++
		// Simulate a stuck client burning past the query budget on the first
		// broadcast (BroadcastCriticalToSession blocks up to HighBlockTimeout).
		time.Sleep(50 * time.Millisecond)
	}, loggateway.NewNoop())
	s.queryTimeout = 5 * time.Millisecond
	s.markTimeout = 200 * time.Millisecond

	s.sweepOnce(context.Background())

	if broadcastCount != 1 {
		t.Fatalf("broadcastCount = %d, want 1 (loop must stop once the query budget is exhausted)", broadcastCount)
	}
	if len(repo.markBatches) != 1 {
		t.Fatalf("MarkPublishedBatch calls = %d, want 1", len(repo.markBatches))
	}
	if len(repo.markBatches[0]) != 1 || repo.markBatches[0][0] != "r1" {
		t.Fatalf("marked ids = %v, want [r1] (only the broadcast row marked; tail left pending)", repo.markBatches[0])
	}
}

func TestEventOutboxSweeper_SweepOnceListErrorIsSilent(t *testing.T) {
	t.Parallel()
	repo := &fakePendingOutbox{listErr: errors.New("scan down")}
	var broadcastCount int
	s := newEventOutboxSweeper(repo, func(string, []byte) { broadcastCount++ }, loggateway.NewNoop())

	// Must not panic; nothing is broadcast or marked.
	s.sweepOnce(context.Background())
	if broadcastCount != 0 || len(repo.markCalls) != 0 {
		t.Fatalf("on list error: broadcasts=%d marks=%v, want none", broadcastCount, repo.markCalls)
	}
}

func TestEventOutboxSweeper_StartStopsWithContext(t *testing.T) {
	t.Parallel()
	repo := &fakePendingOutbox{}
	s := newEventOutboxSweeper(repo, func(string, []byte) {}, loggateway.NewNoop())
	s.interval = 20 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	s.Start(ctx)
	defer cancel()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		repo.mu.Lock()
		n := len(repo.pendingSeen)
		repo.mu.Unlock()
		if n >= 2 {
			return // loop ticks observed
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("sweeper loop did not tick within 2s")
}
