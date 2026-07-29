package data

import (
	"context"
	"testing"
	"time"

	"aranea-agents/internal/data/testhelper"
	"aranea-agents/pkg/loggateway"
)

// setupMonitorEventsRepo builds a Data with the raw-SQL monitor_events table
// mirroring production DDL (see memory_helpers.go schema patch).
func setupMonitorEventsRepo(t *testing.T) *monitorRepo {
	t.Helper()
	client, db := testhelper.SetupTestPG(t)
	ctx := context.Background()
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS monitor_events (
		id TEXT PRIMARY KEY,
		event_key TEXT NOT NULL DEFAULT '',
		name TEXT NOT NULL DEFAULT '',
		description TEXT NOT NULL DEFAULT '',
		status TEXT NOT NULL DEFAULT 'ok',
		metadata_json TEXT NOT NULL DEFAULT '{}',
		created_at TEXT NOT NULL DEFAULT '',
		updated_at TEXT NOT NULL DEFAULT '',
		deleted_at TEXT NOT NULL DEFAULT ''
	)`); err != nil {
		t.Fatalf("create monitor_events: %v", err)
	}
	d := &Data{
		entClient:  client,
		readClient: client,
		rw:         NewReadWriteClient(client, client),
		rawDB:      db,
		readDB:     db,
		rwDB:       NewReadWriteDB(db, db),
		lg:         loggateway.NewNoop(),
		dialect:    DialectPostgres,
	}
	return &monitorRepo{data: d}
}

func insertMonitorEventRow(t *testing.T, r *monitorRepo, id string, createdAt time.Time) {
	t.Helper()
	created := createdAt.UTC().Format(time.RFC3339Nano)
	if _, err := r.data.rawDB.ExecContext(context.Background(),
		`INSERT INTO monitor_events (id, event_key, name, status, created_at, updated_at)
		 VALUES ($1, 'runner.completion', $1, 'ok', $2, $2)`, id, created); err != nil {
		t.Fatalf("insert event %s: %v", id, err)
	}
}

func countMonitorEventRows(t *testing.T, r *monitorRepo) int {
	t.Helper()
	var n int
	if err := r.data.rawDB.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM monitor_events`).Scan(&n); err != nil {
		t.Fatalf("count events: %v", err)
	}
	return n
}

// 保留策略：早于 cutoff 的行物理删除，新行保留。
func TestDeleteMonitorEventsOlderThan(t *testing.T) {
	r := setupMonitorEventsRepo(t)
	ctx := context.Background()
	now := time.Now()

	insertMonitorEventRow(t, r, "ev-old-1", now.Add(-31*24*time.Hour))
	insertMonitorEventRow(t, r, "ev-old-2", now.Add(-30*24*time.Hour-time.Minute))
	insertMonitorEventRow(t, r, "ev-new", now.Add(-time.Hour))

	n, err := r.DeleteMonitorEventsOlderThan(ctx, now.Add(-30*24*time.Hour))
	if err != nil {
		t.Fatalf("DeleteMonitorEventsOlderThan: %v", err)
	}
	if n != 2 {
		t.Errorf("deleted = %d, want 2", n)
	}
	if got := countMonitorEventRows(t, r); got != 1 {
		t.Errorf("remaining = %d, want 1 (ev-new)", got)
	}
}
