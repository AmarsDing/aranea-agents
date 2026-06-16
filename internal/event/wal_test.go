package event

import (
	"context"
	"database/sql"
	"encoding/json"
	"sync"
	"testing"
	"time"

	_ "github.com/glebarez/go-sqlite/compat"

	"aranea-agents/internal/event/contract"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func newTestWAL(t *testing.T) *EventWAL {
	t.Helper()
	db := openTestDB(t)
	w, err := NewEventWAL(db, nil)
	if err != nil {
		t.Fatalf("new test wal: %v", err)
	}
	return w
}

func TestEventWAL_WriteBeforePublish_CriticalEvent(t *testing.T) {
	w := newTestWAL(t)
	ctx := context.Background()

	env := contract.NewEnvelope(contract.EnvelopeTypeToolResult, "agent", "sess-1")
	published := false
	err := w.WriteBeforePublish(ctx, env, func() { published = true })
	if err != nil {
		t.Fatalf("WriteBeforePublish: %v", err)
	}
	if !published {
		t.Error("publish callback was not called for critical event")
	}

	// Verify WAL entry exists
	var count int
	row := w.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM event_wal WHERE id = ?`, env.ID)
	if err := row.Scan(&count); err != nil {
		t.Fatalf("query wal: %v", err)
	}
	if count != 1 {
		t.Errorf("WAL entry count = %d, want 1", count)
	}

	// Verify it was marked published
	var publishedFlag int
	row = w.db.QueryRowContext(ctx, `SELECT published FROM event_wal WHERE id = ?`, env.ID)
	if err := row.Scan(&publishedFlag); err != nil {
		t.Fatalf("query published flag: %v", err)
	}
	if publishedFlag != 1 {
		t.Errorf("published flag = %d, want 1", publishedFlag)
	}
}

func TestEventWAL_WriteBeforePublish_NonCriticalEvent(t *testing.T) {
	w := newTestWAL(t)
	ctx := context.Background()

	env := contract.NewEnvelope(contract.EnvelopeTypeTextDelta, "agent", "sess-1")
	published := false
	err := w.WriteBeforePublish(ctx, env, func() { published = true })
	if err != nil {
		t.Fatalf("WriteBeforePublish: %v", err)
	}
	if !published {
		t.Error("publish callback was not called for non-critical event")
	}

	// Verify NO WAL entry exists
	var count int
	row := w.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM event_wal`)
	if err := row.Scan(&count); err != nil {
		t.Fatalf("query wal: %v", err)
	}
	if count != 0 {
		t.Errorf("WAL entry count = %d, want 0 for non-critical event", count)
	}
}

func TestEventWAL_WriteBeforePublish_Idempotency(t *testing.T) {
	w := newTestWAL(t)
	ctx := context.Background()

	env := contract.NewEnvelope(contract.EnvelopeTypeToolResult, "agent", "sess-1")
	callCount := 0
	err := w.WriteBeforePublish(ctx, env, func() { callCount++ })
	if err != nil {
		t.Fatalf("first WriteBeforePublish: %v", err)
	}

	// Write the same event again (same ID)
	err = w.WriteBeforePublish(ctx, env, func() { callCount++ })
	if err != nil {
		t.Fatalf("second WriteBeforePublish: %v", err)
	}

	// Publish callback should be called twice (WAL doesn't gate publish on idempotency)
	if callCount != 2 {
		t.Errorf("publish call count = %d, want 2", callCount)
	}

	// But only one WAL row should exist (INSERT OR IGNORE)
	var count int
	row := w.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM event_wal WHERE id = ?`, env.ID)
	if err := row.Scan(&count); err != nil {
		t.Fatalf("query wal: %v", err)
	}
	if count != 1 {
		t.Errorf("WAL entry count = %d, want 1 (idempotent)", count)
	}
}

func TestEventWAL_Recover_UnpublishedEvents(t *testing.T) {
	w := newTestWAL(t)
	ctx := context.Background()

	// Insert an unpublished critical event directly into WAL
	env := contract.NewEnvelope(contract.EnvelopeTypeToolResult, "agent", "sess-1")
	raw, _ := marshalEnvelope(t, &env)
	_, err := w.db.ExecContext(ctx,
		`INSERT INTO event_wal (id, envelope_json, created_at, published) VALUES (?, ?, ?, 0)`,
		env.ID, string(raw), time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		t.Fatalf("insert test data: %v", err)
	}

	// Recover with a mock bus
	bus := &mockBus{published: make([]contract.Envelope, 0)}
	w.Recover(ctx, bus, nil)

	if len(bus.published) != 1 {
		t.Fatalf("recovered events = %d, want 1", len(bus.published))
	}
	if bus.published[0].ID != env.ID {
		t.Errorf("recovered event ID = %q, want %q", bus.published[0].ID, env.ID)
	}

	// Verify WAL entry is now marked published
	var publishedFlag int
	row := w.db.QueryRowContext(ctx, `SELECT published FROM event_wal WHERE id = ?`, env.ID)
	if err := row.Scan(&publishedFlag); err != nil {
		t.Fatalf("query published flag: %v", err)
	}
	if publishedFlag != 1 {
		t.Errorf("published flag after recovery = %d, want 1", publishedFlag)
	}
}

func TestEventWAL_Recover_SkipsExistingInEventStore(t *testing.T) {
	w := newTestWAL(t)
	ctx := context.Background()

	env := contract.NewEnvelope(contract.EnvelopeTypeToolResult, "agent", "sess-1")
	raw, _ := marshalEnvelope(t, &env)
	_, err := w.db.ExecContext(ctx,
		`INSERT INTO event_wal (id, envelope_json, created_at, published) VALUES (?, ?, ?, 0)`,
		env.ID, string(raw), time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		t.Fatalf("insert test data: %v", err)
	}

	// EventStore that says the event already exists
	store := &mockExistChecker{existing: map[string]bool{env.ID: true}}
	bus := &mockBus{published: make([]contract.Envelope, 0)}
	w.Recover(ctx, bus, store)

	if len(bus.published) != 0 {
		t.Errorf("recovered events = %d, want 0 (event already in store)", len(bus.published))
	}

	// Should still be marked as published
	var publishedFlag int
	row := w.db.QueryRowContext(ctx, `SELECT published FROM event_wal WHERE id = ?`, env.ID)
	if err := row.Scan(&publishedFlag); err != nil {
		t.Fatalf("query published flag: %v", err)
	}
	if publishedFlag != 1 {
		t.Errorf("published flag = %d, want 1 (marked published after skip)", publishedFlag)
	}
}

func TestEventWAL_Recover_MultipleUnpublished(t *testing.T) {
	w := newTestWAL(t)
	ctx := context.Background()

	// Insert 3 unpublished events
	for i := 0; i < 3; i++ {
		env := contract.NewEnvelope(contract.EnvelopeTypeToolResult, "agent", "sess-1")
		raw, _ := marshalEnvelope(t, &env)
		_, err := w.db.ExecContext(ctx,
			`INSERT INTO event_wal (id, envelope_json, created_at, published) VALUES (?, ?, ?, 0)`,
			env.ID, string(raw), time.Now().UTC().Format(time.RFC3339Nano))
		if err != nil {
			t.Fatalf("insert test data %d: %v", i, err)
		}
	}

	bus := &mockBus{published: make([]contract.Envelope, 0)}
	w.Recover(ctx, bus, nil)

	if len(bus.published) != 3 {
		t.Errorf("recovered events = %d, want 3", len(bus.published))
	}
}

func TestEventWAL_PurgePublished(t *testing.T) {
	w := newTestWAL(t)
	ctx := context.Background()

	// Insert a published event (old)
	env := contract.NewEnvelope(contract.EnvelopeTypeToolResult, "agent", "sess-1")
	raw, _ := marshalEnvelope(t, &env)
	oldTime := time.Now().UTC().Add(-2 * time.Hour).Format(time.RFC3339Nano)
	_, err := w.db.ExecContext(ctx,
		`INSERT INTO event_wal (id, envelope_json, created_at, published, published_at) VALUES (?, ?, ?, 1, ?)`,
		env.ID, string(raw), oldTime, time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		t.Fatalf("insert published event: %v", err)
	}

	// Insert an unpublished event (should NOT be purged)
	env2 := contract.NewEnvelope(contract.EnvelopeTypeError, "agent", "sess-2")
	raw2, _ := marshalEnvelope(t, &env2)
	_, err = w.db.ExecContext(ctx,
		`INSERT INTO event_wal (id, envelope_json, created_at, published) VALUES (?, ?, ?, 0)`,
		env2.ID, string(raw2), time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		t.Fatalf("insert unpublished event: %v", err)
	}

	// Purge entries older than 1 hour
	purged, err := w.PurgePublished(ctx, 1*time.Hour)
	if err != nil {
		t.Fatalf("PurgePublished: %v", err)
	}
	if purged != 1 {
		t.Errorf("purged count = %d, want 1", purged)
	}

	// Verify only the unpublished event remains
	var count int
	row := w.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM event_wal`)
	if err := row.Scan(&count); err != nil {
		t.Fatalf("query count: %v", err)
	}
	if count != 1 {
		t.Errorf("remaining entries = %d, want 1", count)
	}
}

func TestEventWAL_Recover_NilWAL(t *testing.T) {
	var w *EventWAL
	// Should not panic
	w.Recover(context.Background(), nil, nil)
}

func TestEventWAL_PurgePublished_NilWAL(t *testing.T) {
	var w *EventWAL
	n, err := w.PurgePublished(context.Background(), time.Hour)
	if err != nil {
		t.Fatalf("PurgePublished on nil: %v", err)
	}
	if n != 0 {
		t.Errorf("purged = %d, want 0", n)
	}
}

func TestEventWAL_markPublished_Synchronous(t *testing.T) {
	w := newTestWAL(t)
	ctx := context.Background()

	env := contract.NewEnvelope(contract.EnvelopeTypeToolResult, "agent", "sess-1")
	raw, _ := marshalEnvelope(t, &env)
	_, err := w.db.ExecContext(ctx,
		`INSERT INTO event_wal (id, envelope_json, created_at, published) VALUES (?, ?, ?, 0)`,
		env.ID, string(raw), time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		t.Fatalf("insert test data: %v", err)
	}

	err = w.storage.MarkPublished(ctx, env.ID, time.Now().UTC())
	if err != nil {
		t.Fatalf("MarkPublished: %v", err)
	}

	var publishedFlag int
	var publishedAt sql.NullString
	row := w.db.QueryRowContext(ctx, `SELECT published, published_at FROM event_wal WHERE id = ?`, env.ID)
	if err := row.Scan(&publishedFlag, &publishedAt); err != nil {
		t.Fatalf("query: %v", err)
	}
	if publishedFlag != 1 {
		t.Errorf("published flag = %d, want 1", publishedFlag)
	}
	if !publishedAt.Valid || publishedAt.String == "" {
		t.Error("published_at should be set after markPublished")
	}
}

// --- test helpers ---

type mockBus struct {
	mu        sync.Mutex
	published []contract.Envelope
}

func (b *mockBus) Publish(_ context.Context, env contract.Envelope) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.published = append(b.published, env)
}

func (b *mockBus) Subscribe(_ contract.SubscribeOptions) (<-chan contract.Envelope, func()) {
	ch := make(chan contract.Envelope)
	return ch, func() { close(ch) }
}

func (b *mockBus) DropCount() uint64 { return 0 }

type mockExistChecker struct {
	existing map[string]bool
}

func (m *mockExistChecker) Exists(_ context.Context, eventID string) bool {
	return m.existing[eventID]
}

func marshalEnvelope(t *testing.T, env *contract.Envelope) ([]byte, error) {
	t.Helper()
	return json.Marshal(env)
}
