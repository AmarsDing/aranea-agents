package wal

import (
	"context"
	"testing"
	"time"
)

type testEvent struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	Data string `json:"data"`
}

type testLogger struct {
	warns []string
	infos []string
}

func (l *testLogger) Warn(msg string, kv ...any) {
	l.warns = append(l.warns, msg)
}

func (l *testLogger) Info(msg string, kv ...any) {
	l.infos = append(l.infos, msg)
}

func isCriticalTestEvent(event testEvent) bool {
	return event.Type == "critical"
}

func TestWAL_WriteBeforePublish_CriticalEvent(t *testing.T) {
	storage := NewMemoryStorage()
	lg := &testLogger{}
	w, err := New[testEvent](storage, isCriticalTestEvent, lg)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	published := false
	err = w.WriteBeforePublish(context.Background(), testEvent{
		ID:   "evt-1",
		Type: "critical",
		Data: "test",
	}, "evt-1", func() {
		published = true
	})
	if err != nil {
		t.Fatalf("WriteBeforePublish() error: %v", err)
	}
	if !published {
		t.Error("expected publish callback to be called")
	}

	// Verify WAL entry was created and marked published
	entries, _ := storage.ListUnpublished(context.Background())
	if len(entries) != 0 {
		t.Errorf("expected 0 unpublished entries, got %d", len(entries))
	}
}

func TestWAL_WriteBeforePublish_NonCriticalEvent(t *testing.T) {
	storage := NewMemoryStorage()
	lg := &testLogger{}
	w, err := New[testEvent](storage, isCriticalTestEvent, lg)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	published := false
	err = w.WriteBeforePublish(context.Background(), testEvent{
		ID:   "evt-2",
		Type: "info",
		Data: "test",
	}, "evt-2", func() {
		published = true
	})
	if err != nil {
		t.Fatalf("WriteBeforePublish() error: %v", err)
	}
	if !published {
		t.Error("expected publish callback to be called")
	}

	// Non-critical events should NOT be stored in WAL
	storage.mu.RLock()
	total := len(storage.entries)
	storage.mu.RUnlock()
	if total != 0 {
		t.Errorf("expected 0 WAL entries for non-critical event, got %d", total)
	}
}

func TestWAL_Recover(t *testing.T) {
	storage := NewMemoryStorage()
	lg := &testLogger{}
	w, err := New[testEvent](storage, isCriticalTestEvent, lg)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	// Manually insert an unpublished entry
	storage.Insert(context.Background(), Entry{
		ID:        "evt-recover",
		EventJSON: `{"id":"evt-recover","type":"critical","data":"recovered"}`,
		CreatedAt: time.Now().UTC(),
		Published: false,
	})

	// Recover
	var recovered []testEvent
	w.Recover(context.Background(), func(event testEvent) {
		recovered = append(recovered, event)
	}, nil)

	if len(recovered) != 1 {
		t.Fatalf("expected 1 recovered event, got %d", len(recovered))
	}
	if recovered[0].ID != "evt-recover" {
		t.Errorf("expected ID 'evt-recover', got %q", recovered[0].ID)
	}
}

func TestWAL_Recover_WithExistChecker(t *testing.T) {
	storage := NewMemoryStorage()
	lg := &testLogger{}
	w, err := New[testEvent](storage, isCriticalTestEvent, lg)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	// Insert an unpublished entry
	storage.Insert(context.Background(), Entry{
		ID:        "evt-exists",
		EventJSON: `{"id":"evt-exists","type":"critical","data":"already-processed"}`,
		CreatedAt: time.Now().UTC(),
		Published: false,
	})

	// ExistChecker says the event already exists
	checker := &mockExistChecker{existing: map[string]bool{"evt-exists": true}}

	var recovered []testEvent
	w.Recover(context.Background(), func(event testEvent) {
		recovered = append(recovered, event)
	}, checker)

	if len(recovered) != 0 {
		t.Errorf("expected 0 recovered events (already exists), got %d", len(recovered))
	}
}

func TestWAL_PurgePublished(t *testing.T) {
	storage := NewMemoryStorage()
	lg := &testLogger{}
	w, err := New[testEvent](storage, isCriticalTestEvent, lg)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	// Insert and publish a critical event
	w.WriteBeforePublish(context.Background(), testEvent{
		ID:   "evt-purge",
		Type: "critical",
		Data: "test",
	}, "evt-purge", func() {})

	// Purge published entries older than 1 hour TTL
	// Since the entry was just created, we need to manipulate the storage
	// to simulate an old entry.
	storage.mu.Lock()
	for id, e := range storage.entries {
		e.CreatedAt = time.Now().UTC().Add(-2 * time.Hour)
		storage.entries[id] = e
	}
	storage.mu.Unlock()

	purged, err := w.PurgePublished(context.Background(), 1*time.Hour)
	if err != nil {
		t.Fatalf("PurgePublished() error: %v", err)
	}
	if purged != 1 {
		t.Errorf("expected 1 purged entry, got %d", purged)
	}
}

func TestWAL_NilStorage(t *testing.T) {
	_, err := New[testEvent](nil, isCriticalTestEvent, nil)
	if err == nil {
		t.Error("expected error for nil storage")
	}
}

func TestWAL_NilIsCritical(t *testing.T) {
	storage := NewMemoryStorage()
	w, err := New[testEvent](storage, nil, nil) // nil isCritical = all events are critical
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	published := false
	err = w.WriteBeforePublish(context.Background(), testEvent{
		ID:   "evt-any",
		Type: "info",
		Data: "test",
	}, "evt-any", func() {
		published = true
	})
	if err != nil {
		t.Fatalf("WriteBeforePublish() error: %v", err)
	}
	if !published {
		t.Error("expected publish callback to be called")
	}

	// With nil isCritical, ALL events should be stored in WAL
	storage.mu.RLock()
	total := len(storage.entries)
	storage.mu.RUnlock()
	if total != 1 {
		t.Errorf("expected 1 WAL entry (nil isCritical = all critical), got %d", total)
	}
}

type mockExistChecker struct {
	existing map[string]bool
}

func (m *mockExistChecker) Exists(ctx context.Context, eventID string) bool {
	return m.existing[eventID]
}
