package data

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/testhelper"
	"aranea-agents/pkg/loggateway"
)

func newEventOutboxTestRepo(t *testing.T) (biz.EventDeliveryOutboxRepo, *sql.DB) {
	t.Helper()
	ctx := context.Background()
	db := testhelper.SetupTestPGRaw(t)
	// DDL mirrors sql/migrations/20261010_event_delivery_outbox.sql (Postgres dialect).
	for _, s := range []string{
		`CREATE TABLE IF NOT EXISTS event_delivery_outbox (
			id TEXT PRIMARY KEY,
			session_id TEXT NOT NULL,
			seq INTEGER NOT NULL,
			event_id TEXT NOT NULL,
			kind TEXT NOT NULL DEFAULT '',
			entity_id TEXT NOT NULL DEFAULT '',
			payload BYTEA NOT NULL,
			published_at TEXT,
			created_at TEXT NOT NULL
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_event_delivery_outbox_session_seq
			ON event_delivery_outbox(session_id, seq)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_event_delivery_outbox_session_event_id
			ON event_delivery_outbox(session_id, event_id)`,
		`CREATE INDEX IF NOT EXISTS idx_event_delivery_outbox_session_id
			ON event_delivery_outbox(session_id)`,
		// Mirrors sql/migrations/20261019_event_delivery_outbox_pending_idx.sql.
		`CREATE INDEX IF NOT EXISTS idx_event_delivery_outbox_pending
			ON event_delivery_outbox(created_at) WHERE published_at IS NULL`,
		// Mirrors sql/migrations/20261303_session_seq_counters.sql（D7-1）。
		`CREATE TABLE IF NOT EXISTS session_seq_counters (
			session_id TEXT PRIMARY KEY,
			next_seq BIGINT NOT NULL
		)`,
		// Mirrors sql/migrations/20261304_event_delivery_outbox_claim.sql（D7-2）。
		`ALTER TABLE event_delivery_outbox ADD COLUMN IF NOT EXISTS claimed_by TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE event_delivery_outbox ADD COLUMN IF NOT EXISTS claimed_at TEXT NOT NULL DEFAULT ''`,
	} {
		if _, err := db.ExecContext(ctx, s); err != nil {
			t.Fatalf("outbox schema: %v", err)
		}
	}
	d := &Data{
		rawDB:   db,
		readDB:  db,
		rwDB:    NewReadWriteDB(db, db),
		lg:      loggateway.NewNoop(),
		dialect: DialectPostgres,
	}
	return NewEventDeliveryOutboxRepo(d, loggateway.NewNoop()), db
}

func TestEventDeliveryOutbox_InsertAndListAfter(t *testing.T) {
	ctx := context.Background()
	repo, _ := newEventOutboxTestRepo(t)

	now := time.Now().UTC()
	rows := []biz.EventDeliveryOutboxRow{
		{
			ID: "row-1", SessionID: "sess-1", Seq: 1, EventID: "v2:sess-1:1:task.completed:t1",
			Kind: "task.completed", EntityID: "t1", Payload: []byte(`{"type":"v2_event","event_id":"v2:sess-1:1:task.completed:t1"}`),
			CreatedAt: now,
		},
		{
			ID: "row-2", SessionID: "sess-1", Seq: 2, EventID: "v2:sess-1:2:task.failed:t2",
			Kind: "task.failed", EntityID: "t2", Payload: []byte(`{"type":"v2_event","event_id":"v2:sess-1:2:task.failed:t2"}`),
			CreatedAt: now,
		},
		{
			ID: "row-3", SessionID: "sess-1", Seq: 3, EventID: "v2:sess-1:3:system.notice:sess-1",
			Kind: "system.notice", EntityID: "sess-1", Payload: []byte(`{"type":"v2_event","event_id":"v2:sess-1:3:system.notice:sess-1"}`),
			CreatedAt: now,
		},
		{
			ID: "row-other", SessionID: "sess-other", Seq: 1, EventID: "v2:sess-other:1:task.completed:x",
			Kind: "task.completed", EntityID: "x", Payload: []byte(`{"type":"v2_event"}`),
			CreatedAt: now,
		},
	}
	for _, row := range rows {
		if err := repo.Insert(ctx, row); err != nil {
			t.Fatalf("Insert(%s): %v", row.ID, err)
		}
	}

	// Cursor by event_id: after seq=1 → expect seq 2 and 3 only (same session).
	got, err := repo.ListAfter(ctx, "sess-1", "v2:sess-1:1:task.completed:t1", 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("ListAfter by event_id: got %d rows, want 2", len(got))
	}
	if got[0].Seq != 2 || got[1].Seq != 3 {
		t.Fatalf("ListAfter order/filter = %+v, want seq 2 then 3", got)
	}
	if got[0].EventID != "v2:sess-1:2:task.failed:t2" {
		t.Fatalf("first event_id = %q", got[0].EventID)
	}

	// Cursor by afterSeq directly.
	gotSeq, err := repo.ListAfter(ctx, "sess-1", "", 2, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(gotSeq) != 1 || gotSeq[0].Seq != 3 {
		t.Fatalf("ListAfter by seq: got %+v, want single seq=3", gotSeq)
	}

	// Unknown event_id → empty (no replay of entire history).
	unknown, err := repo.ListAfter(ctx, "sess-1", "missing-event-id", 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(unknown) != 0 {
		t.Fatalf("unknown event_id should return empty, got %d", len(unknown))
	}

	// Duplicate (session_id, seq) is ignored (ON CONFLICT DO NOTHING).
	if err := repo.Insert(ctx, biz.EventDeliveryOutboxRow{
		ID: "row-dup", SessionID: "sess-1", Seq: 1, EventID: "v2:sess-1:1:dup",
		Kind: "task.completed", Payload: []byte(`{}`), CreatedAt: now,
	}); err != nil {
		t.Fatalf("duplicate insert: %v", err)
	}
	all, err := repo.ListAfter(ctx, "sess-1", "", 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 {
		t.Fatalf("after duplicate ignore: got %d rows, want 3", len(all))
	}
}

func TestEventDeliveryOutbox_NilRepoNoPanic(t *testing.T) {
	ctx := context.Background()
	var typedNil *eventDeliveryOutboxRepo
	if err := typedNil.Insert(ctx, biz.EventDeliveryOutboxRow{}); err != nil {
		t.Fatalf("typed nil Insert: %v", err)
	}
	if err := typedNil.MarkPublished(ctx, "x", time.Now()); err != nil {
		t.Fatalf("typed nil MarkPublished: %v", err)
	}
	if err := typedNil.MarkPublishedBatch(ctx, []string{"x"}, time.Now()); err != nil {
		t.Fatalf("typed nil MarkPublishedBatch: %v", err)
	}
	if rows, err := typedNil.ListAfter(ctx, "s", "e", 0, 10); err != nil || rows != nil {
		t.Fatalf("typed nil ListAfter: rows=%v err=%v", rows, err)
	}
	iface := NewEventDeliveryOutboxRepo(nil, nil)
	if iface != nil {
		t.Fatal("NewEventDeliveryOutboxRepo(nil) should return nil interface")
	}
}

func TestEventDeliveryOutbox_MarkPublished(t *testing.T) {
	ctx := context.Background()
	repo, _ := newEventOutboxTestRepo(t)

	now := time.Now().UTC()
	if err := repo.Insert(ctx, biz.EventDeliveryOutboxRow{
		ID: "pub-1", SessionID: "sess-1", Seq: 5, EventID: "evt-5",
		Kind: "task.completed", Payload: []byte(`{}`), CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	publishedAt := now.Add(time.Second)
	if err := repo.MarkPublished(ctx, "pub-1", publishedAt); err != nil {
		t.Fatal(err)
	}
	got, err := repo.ListAfter(ctx, "sess-1", "", 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].PublishedAt == nil {
		t.Fatalf("expected PublishedAt set, got %+v", got)
	}
}

func TestEventDeliveryOutbox_MarkPublishedBatch(t *testing.T) {
	ctx := context.Background()
	repo, _ := newEventOutboxTestRepo(t)

	now := time.Now().UTC()
	old := now.Add(-60 * time.Second)
	rows := []biz.EventDeliveryOutboxRow{
		{ID: "b-1", SessionID: "sess-1", Seq: 1, EventID: "evt-b1", Kind: "task.completed", Payload: []byte(`{}`), CreatedAt: old},
		{ID: "b-2", SessionID: "sess-1", Seq: 2, EventID: "evt-b2", Kind: "task.failed", Payload: []byte(`{}`), CreatedAt: old},
		{ID: "b-3", SessionID: "sess-1", Seq: 3, EventID: "evt-b3", Kind: "task.completed", Payload: []byte(`{}`), CreatedAt: old},
	}
	for _, row := range rows {
		if err := repo.Insert(ctx, row); err != nil {
			t.Fatalf("Insert(%s): %v", row.ID, err)
		}
	}

	// Empty / blank-only lists are no-ops and must not error.
	if err := repo.MarkPublishedBatch(ctx, nil, now); err != nil {
		t.Fatalf("batch nil: %v", err)
	}
	if err := repo.MarkPublishedBatch(ctx, []string{"", "  "}, now); err != nil {
		t.Fatalf("batch blanks: %v", err)
	}

	// Duplicates collapse; already-published rows keep their first timestamp.
	publishedAt := now.Add(time.Second)
	if err := repo.MarkPublishedBatch(ctx, []string{"b-1", "b-2", "b-1"}, publishedAt); err != nil {
		t.Fatal(err)
	}
	got, err := repo.ListPending(ctx, now.Add(-time.Second), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "b-3" {
		t.Fatalf("ListPending after batch: got %+v, want only b-3 pending", got)
	}
	all, err := repo.ListAfter(ctx, "sess-1", "", 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 || all[0].PublishedAt == nil || all[1].PublishedAt == nil || all[2].PublishedAt != nil {
		t.Fatalf("published state = %+v, want b-1/b-2 published and b-3 pending", all)
	}

	// Second batch must not flip b-3's state of b-1/b-2 nor touch unknown ids.
	if err := repo.MarkPublishedBatch(ctx, []string{"b-3", "missing"}, now.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	remaining, err := repo.ListPending(ctx, now.Add(-time.Second), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 0 {
		t.Fatalf("ListPending after second batch: got %+v, want empty", remaining)
	}
}

func TestEventDeliveryOutbox_ListPending(t *testing.T) {
	ctx := context.Background()
	repo, _ := newEventOutboxTestRepo(t)

	now := time.Now().UTC()
	old := now.Add(-60 * time.Second)
	older := now.Add(-120 * time.Second)
	rows := []biz.EventDeliveryOutboxRow{
		// Stuck: old + unpublished → swept.
		{ID: "p-old-2", SessionID: "sess-1", Seq: 2, EventID: "evt-old-2", Kind: "task.failed", EntityID: "t2", Payload: []byte(`{}`), CreatedAt: old},
		{ID: "p-older-1", SessionID: "sess-1", Seq: 1, EventID: "evt-older-1", Kind: "task.completed", EntityID: "t1", Payload: []byte(`{}`), CreatedAt: older},
		// Younger than cutoff → not yet considered stuck.
		{ID: "p-new", SessionID: "sess-1", Seq: 3, EventID: "evt-new", Kind: "task.completed", EntityID: "t3", Payload: []byte(`{}`), CreatedAt: now},
		// Old but already published → excluded.
		{ID: "p-published", SessionID: "sess-1", Seq: 4, EventID: "evt-published", Kind: "task.completed", EntityID: "t4", Payload: []byte(`{}`), CreatedAt: old},
		// Old + unpublished in another session — sweep is global (per-tick
		// broadcast is routed by row.SessionID).
		{ID: "p-other", SessionID: "sess-other", Seq: 1, EventID: "evt-other", Kind: "task.completed", EntityID: "x", Payload: []byte(`{}`), CreatedAt: old},
	}
	for _, row := range rows {
		if err := repo.Insert(ctx, row); err != nil {
			t.Fatalf("Insert(%s): %v", row.ID, err)
		}
	}
	if err := repo.MarkPublished(ctx, "p-published", old.Add(time.Second)); err != nil {
		t.Fatal(err)
	}

	got, err := repo.ListPending(ctx, now.Add(-10*time.Second), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("ListPending: got %d rows (%+v), want 3", len(got), got)
	}
	// Oldest first across sessions.
	if got[0].ID != "p-older-1" || got[1].ID != "p-old-2" || got[2].ID != "p-other" {
		t.Fatalf("ListPending order = %s,%s,%s; want p-older-1,p-old-2,p-other", got[0].ID, got[1].ID, got[2].ID)
	}

	// Limit respected.
	limited, err := repo.ListPending(ctx, now.Add(-10*time.Second), 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(limited) != 2 || limited[0].ID != "p-older-1" {
		t.Fatalf("ListPending limit=2: got %+v", limited)
	}

	// Zero cutoff = no age filter (all unpublished).
	all, err := repo.ListPending(ctx, time.Time{}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 4 {
		t.Fatalf("ListPending zero cutoff: got %d rows, want 4 (all unpublished)", len(all))
	}

	// Typed-nil repo must not panic.
	var typedNil *eventDeliveryOutboxRepo
	if rows, err := typedNil.ListPending(ctx, now, 10); err != nil || rows != nil {
		t.Fatalf("typed nil ListPending: rows=%v err=%v", rows, err)
	}
}
