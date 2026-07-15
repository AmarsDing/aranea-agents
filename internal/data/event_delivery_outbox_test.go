package data

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"

	_ "github.com/glebarez/go-sqlite/compat"
)

func newEventOutboxTestRepo(t *testing.T, dbName string) (biz.EventDeliveryOutboxRepo, *sql.DB) {
	t.Helper()
	ctx := context.Background()
	db, err := sql.Open("sqlite", "file:"+dbName+"?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := EnsureEventDeliveryOutboxSchema(ctx, db); err != nil {
		t.Fatal(err)
	}
	d := &Data{
		rawDB:   db,
		readDB:  db,
		rwDB:    NewReadWriteDB(db, db),
		lg:      loggateway.NewNoop(),
		dialect: DialectSQLite,
	}
	return NewEventDeliveryOutboxRepo(d, loggateway.NewNoop()), db
}

func TestEventDeliveryOutbox_InsertAndListAfter(t *testing.T) {
	ctx := context.Background()
	repo, _ := newEventOutboxTestRepo(t, "event-outbox-insert-list")

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

	// Duplicate (session_id, seq) is ignored (INSERT OR IGNORE).
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
	repo, _ := newEventOutboxTestRepo(t, "event-outbox-mark-published")

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
