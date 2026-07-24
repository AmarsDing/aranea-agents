package session

import (
	"context"
	"testing"

	"aranea-agents/internal/data"
	"aranea-agents/internal/data/testhelper"
	"aranea-agents/pkg/ctxuser"
	loggateway "aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/trpcscope"
)

func TestRunnerRollbackStoreSoftDeletesEventsAfterBoundary(t *testing.T) {
	db := testhelper.SetupTestPGRaw(t)
	// DDL mirrors cmd/migrate-sqlite-to-postgres/framework_schema.go (Postgres dialect).
	_, err := db.Exec(`CREATE TABLE trpc_session_events (
		id BIGSERIAL PRIMARY KEY,
		app_name VARCHAR(255) NOT NULL,
		user_id VARCHAR(255) NOT NULL,
		session_id VARCHAR(255) NOT NULL,
		event JSONB NOT NULL,
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		expires_at TIMESTAMP DEFAULT NULL,
		deleted_at TIMESTAMP DEFAULT NULL
	)`)
	if err != nil {
		t.Fatal(err)
	}
	ctx := ctxuser.WithUserID(context.Background(), "u1")
	insertEvent := func() {
		t.Helper()
		if _, err := db.Exec(`INSERT INTO trpc_session_events (app_name, user_id, session_id, event) VALUES ($1, $2, $3, '{}')`, trpcscope.DefaultAppName, "u1", "s1"); err != nil {
			t.Fatal(err)
		}
	}
	lg := loggateway.NewNoop()
	rwdb := data.NewReadWriteDB(db, db)
	dialect := data.DialectPostgres
	insertEvent()
	boundary, err := NewRunnerRollbackStore(rwdb, dialect, lg).MarkBoundary(ctx, "s1", "run-1", "turn-1")
	if err != nil {
		t.Fatal(err)
	}
	insertEvent()
	insertEvent()
	if err := NewRunnerRollbackStore(rwdb, dialect, lg).RollbackToBoundary(context.Background(), "s1", boundary); err != nil {
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
