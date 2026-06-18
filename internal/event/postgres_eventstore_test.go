//go:build integration

package event

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"

	"aranea-agents/internal/event/contract"
	"aranea-agents/pkg/loggateway"
)

// newTestPostgresStore opens a Postgres connection using ARANEA_TEST_POSTGRES_DSN
// and returns a fresh PostgresEventStore with the event_store table truncated.
// Skips the test when the DSN env var is not set.
func newTestPostgresStore(t *testing.T) *PostgresEventStore {
	t.Helper()
	dsn := os.Getenv("ARANEA_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("ARANEA_TEST_POSTGRES_DSN not set; skipping Postgres integration test")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	// Drop any leftover data so each test starts from a clean slate.
	if _, err := db.Exec(`DROP TABLE IF EXISTS event_store`); err != nil {
		t.Fatalf("drop event_store: %v", err)
	}

	store, err := NewPostgresEventStore(db, loggateway.NewNoop())
	if err != nil {
		t.Fatalf("NewPostgresEventStore: %v", err)
	}
	t.Cleanup(func() {
		// Best-effort cleanup; ignore error (connection may already be closed).
		_, _ = db.Exec(`DROP TABLE IF EXISTS event_store`)
	})
	return store
}

func TestPostgresEventStore_EnsureSchema(t *testing.T) {
	store := newTestPostgresStore(t)
	ctx := context.Background()

	// Calling EnsureSchema again must be idempotent (table + indexes already exist).
	if err := store.EnsureSchema(ctx); err != nil {
		t.Fatalf("EnsureSchema (2nd call): %v", err)
	}
	// A third call should still succeed.
	if err := store.EnsureSchema(ctx); err != nil {
		t.Fatalf("EnsureSchema (3rd call): %v", err)
	}

	// Verify the table exists by inserting and reading back a row.
	env := contract.NewEnvelope(contract.EnvelopeTypeTextDelta, "agent", "sess-schema")
	if err := store.Save(ctx, &env); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := store.Replay(ctx, "sess-schema", "", 10)
	if err != nil {
		t.Fatalf("Replay: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("after schema ensure: got %d events, want 1", len(got))
	}
}

func TestPostgresEventStore_Save_Replay(t *testing.T) {
	store := newTestPostgresStore(t)
	ctx := context.Background()

	env1 := contract.NewEnvelope(contract.EnvelopeTypeTextDelta, "agent", "sess-1")
	env1.Content = &contract.EnvelopeContent{Text: "hello"}
	env2 := contract.NewEnvelope(contract.EnvelopeTypeToolResult, "agent", "sess-1")
	env2.Content = &contract.EnvelopeContent{Text: "tool output"}

	// Stagger timestamps so ordering is deterministic.
	env1.Timestamp = time.Now().UTC().Add(-1 * time.Second).Format(time.RFC3339Nano)
	env2.Timestamp = time.Now().UTC().Format(time.RFC3339Nano)

	if err := store.Save(ctx, &env1); err != nil {
		t.Fatalf("Save env1: %v", err)
	}
	if err := store.Save(ctx, &env2); err != nil {
		t.Fatalf("Save env2: %v", err)
	}

	got, err := store.Replay(ctx, "sess-1", "", 100)
	if err != nil {
		t.Fatalf("Replay: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("Replay returned %d events, want 2", len(got))
	}
	if got[0].ID != env1.ID {
		t.Errorf("first event ID = %q, want %q", got[0].ID, env1.ID)
	}
	if got[1].ID != env2.ID {
		t.Errorf("second event ID = %q, want %q", got[1].ID, env2.ID)
	}
	if got[0].Content == nil || got[0].Content.Text != "hello" {
		t.Errorf("first event content text = %v, want %q", got[0].Content, "hello")
	}
}

func TestPostgresEventStore_Save_Idempotent(t *testing.T) {
	store := newTestPostgresStore(t)
	ctx := context.Background()

	env := contract.NewEnvelope(contract.EnvelopeTypeToolResult, "agent", "sess-idem")

	// First save — should succeed.
	if err := store.Save(ctx, &env); err != nil {
		t.Fatalf("first Save: %v", err)
	}
	// Second save with the same event_id — should be a no-op, no error.
	if err := store.Save(ctx, &env); err != nil {
		t.Fatalf("second Save (idempotent): %v", err)
	}
	// Third save — still no error.
	if err := store.Save(ctx, &env); err != nil {
		t.Fatalf("third Save (idempotent): %v", err)
	}

	got, err := store.Replay(ctx, "sess-idem", "", 100)
	if err != nil {
		t.Fatalf("Replay: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("after 3 saves with same ID: got %d events, want 1", len(got))
	}
}

func TestPostgresEventStore_Replay_AfterEventID(t *testing.T) {
	store := newTestPostgresStore(t)
	ctx := context.Background()

	// Insert 3 events with staggered timestamps.
	var envs []contract.Envelope
	for i := 0; i < 3; i++ {
		env := contract.NewEnvelope(contract.EnvelopeTypeTextDelta, "agent", "sess-after")
		env.Timestamp = time.Now().UTC().Add(time.Duration(i) * time.Second).Format(time.RFC3339Nano)
		envs = append(envs, env)
		if err := store.Save(ctx, &env); err != nil {
			t.Fatalf("Save %d: %v", i, err)
		}
	}

	// Replay events after envs[0].ID — should return envs[1] and envs[2].
	got, err := store.Replay(ctx, "sess-after", envs[0].ID, 100)
	if err != nil {
		t.Fatalf("Replay afterEventID: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("Replay afterEventID returned %d events, want 2", len(got))
	}
	if got[0].ID != envs[1].ID {
		t.Errorf("first event after cursor = %q, want %q", got[0].ID, envs[1].ID)
	}
	if got[1].ID != envs[2].ID {
		t.Errorf("second event after cursor = %q, want %q", got[1].ID, envs[2].ID)
	}

	// Replay after envs[2].ID (the last) — should return 0 events.
	got, err = store.Replay(ctx, "sess-after", envs[2].ID, 100)
	if err != nil {
		t.Fatalf("Replay after last: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("Replay after last event returned %d events, want 0", len(got))
	}
}

func TestPostgresEventStore_Replay_Limit(t *testing.T) {
	store := newTestPostgresStore(t)
	ctx := context.Background()

	// Insert 5 events.
	for i := 0; i < 5; i++ {
		env := contract.NewEnvelope(contract.EnvelopeTypeTextDelta, "agent", "sess-limit")
		env.Timestamp = time.Now().UTC().Add(time.Duration(i) * time.Second).Format(time.RFC3339Nano)
		if err := store.Save(ctx, &env); err != nil {
			t.Fatalf("Save %d: %v", i, err)
		}
	}

	// Limit = 2 → only the 2 oldest events (ASC ordering).
	got, err := store.Replay(ctx, "sess-limit", "", 2)
	if err != nil {
		t.Fatalf("Replay limit=2: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("Replay limit=2 returned %d events, want 2", len(got))
	}

	// Limit = 0 → falls back to default (100), returns all 5.
	got, err = store.Replay(ctx, "sess-limit", "", 0)
	if err != nil {
		t.Fatalf("Replay limit=0 (default): %v", err)
	}
	if len(got) != 5 {
		t.Errorf("Replay limit=0 (default) returned %d events, want 5", len(got))
	}

	// Limit = -1 → also falls back to default.
	got, err = store.Replay(ctx, "sess-limit", "", -1)
	if err != nil {
		t.Fatalf("Replay limit=-1 (default): %v", err)
	}
	if len(got) != 5 {
		t.Errorf("Replay limit=-1 (default) returned %d events, want 5", len(got))
	}
}

func TestPostgresEventStore_Cleanup(t *testing.T) {
	store := newTestPostgresStore(t)
	ctx := context.Background()

	// Insert an old event (2 hours ago) and a recent event (now).
	oldEnv := contract.NewEnvelope(contract.EnvelopeTypeTextDelta, "agent", "sess-cleanup")
	oldEnv.Timestamp = time.Now().UTC().Add(-2 * time.Hour).Format(time.RFC3339Nano)
	if err := store.Save(ctx, &oldEnv); err != nil {
		t.Fatalf("Save old: %v", err)
	}

	newEnv := contract.NewEnvelope(contract.EnvelopeTypeTextDelta, "agent", "sess-cleanup")
	newEnv.Timestamp = time.Now().UTC().Format(time.RFC3339Nano)
	if err := store.Save(ctx, &newEnv); err != nil {
		t.Fatalf("Save new: %v", err)
	}

	// Cleanup events older than 1 hour ago — should remove only oldEnv.
	cutoff := time.Now().UTC().Add(-1 * time.Hour)
	if err := store.Cleanup(ctx, cutoff); err != nil {
		t.Fatalf("Cleanup: %v", err)
	}

	got, err := store.Replay(ctx, "sess-cleanup", "", 100)
	if err != nil {
		t.Fatalf("Replay after cleanup: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("after cleanup: got %d events, want 1", len(got))
	}
	if got[0].ID != newEnv.ID {
		t.Errorf("remaining event ID = %q, want %q (new event should survive)", got[0].ID, newEnv.ID)
	}
}

func TestPostgresEventStore_Save_NilEnvelope(t *testing.T) {
	store := newTestPostgresStore(t)
	ctx := context.Background()

	err := store.Save(ctx, nil)
	if err == nil {
		t.Fatal("Save(nil) should return an error")
	}
}

func TestPostgresEventStore_Replay_EmptySession(t *testing.T) {
	store := newTestPostgresStore(t)
	ctx := context.Background()

	// A session with no events should return an empty slice, not nil.
	got, err := store.Replay(ctx, "sess-empty", "", 100)
	if err != nil {
		t.Fatalf("Replay empty session: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("Replay empty session returned %d events, want 0", len(got))
	}
}
