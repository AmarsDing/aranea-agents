package session

import (
	"context"
	"database/sql"
	"testing"

	"aranea-agents/pkg/ctxuser"
	"aranea-agents/pkg/trpcscope"

	_ "github.com/glebarez/go-sqlite/compat"
)

func TestRunnerRollbackStoreSoftDeletesEventsAfterBoundary(t *testing.T) {
	db, err := sql.Open("sqlite", "file:"+t.Name()+"?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, err = db.Exec(`CREATE TABLE trpc_session_events (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		app_name TEXT NOT NULL,
		user_id TEXT NOT NULL,
		session_id TEXT NOT NULL,
		event BLOB NOT NULL,
		created_at INTEGER NOT NULL,
		updated_at INTEGER NOT NULL,
		expires_at INTEGER DEFAULT NULL,
		deleted_at INTEGER DEFAULT NULL
	)`)
	if err != nil {
		t.Fatal(err)
	}
	ctx := ctxuser.WithUserID(context.Background(), "u1")
	insertEvent := func() {
		t.Helper()
		if _, err := db.Exec(`INSERT INTO trpc_session_events (app_name, user_id, session_id, event, created_at, updated_at) VALUES (?, ?, ?, '{}', 1, 1)`, trpcscope.DefaultAppName, "u1", "s1"); err != nil {
			t.Fatal(err)
		}
	}
	insertEvent()
	boundary, err := NewRunnerRollbackStore(db).MarkBoundary(ctx, "s1", "run-1", "turn-1")
	if err != nil {
		t.Fatal(err)
	}
	insertEvent()
	insertEvent()
	if err := NewRunnerRollbackStore(db).RollbackToBoundary(context.Background(), "s1", boundary); err != nil {
		t.Fatal(err)
	}
	var live int
	if err := db.QueryRow(`SELECT COUNT(1) FROM trpc_session_events WHERE deleted_at IS NULL`).Scan(&live); err != nil {
		t.Fatal(err)
	}
	if live != 1 {
		t.Fatalf("live events = %d, want 1", live)
	}
}
