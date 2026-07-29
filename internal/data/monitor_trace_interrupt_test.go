package data

import (
	"context"
	"testing"
	"time"
)

// setupInterruptRepo builds a Data with the raw-SQL monitor_traces +
// monitor_trace_spans tables mirroring production DDL. Reuses the display-name
// test's monitor_traces setup and adds the spans table.
func setupInterruptRepo(t *testing.T) *monitorRepo {
	t.Helper()
	r := setupMonitorTraceNameRepo(t)
	ctx := context.Background()
	if _, err := r.data.rawDB.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS monitor_trace_spans (
		id BIGSERIAL PRIMARY KEY,
		trace_id TEXT NOT NULL,
		span_id TEXT NOT NULL,
		parent_span_id TEXT NOT NULL DEFAULT '',
		kind TEXT NOT NULL,
		name TEXT NOT NULL,
		started_at BIGINT NOT NULL,
		ended_at BIGINT NOT NULL DEFAULT 0,
		status TEXT NOT NULL DEFAULT 'running',
		attributes_json TEXT NOT NULL DEFAULT '',
		error_json TEXT NOT NULL DEFAULT '',
		UNIQUE(trace_id, span_id)
	)`); err != nil {
		t.Fatalf("create monitor_trace_spans: %v", err)
	}
	return r
}

// insertInterruptTraceRow inserts a monitor_traces row with an explicit
// created_at (InsertMonitorTrace always stamps time.Now, so raw SQL is used).
func insertInterruptTraceRow(t *testing.T, r *monitorRepo, id, status string, createdAt time.Time) {
	t.Helper()
	created := createdAt.UTC().Format(time.RFC3339)
	if _, err := r.data.rawDB.ExecContext(context.Background(),
		`INSERT INTO monitor_traces (id, trace_key, name, status, created_at, updated_at)
		 VALUES ($1, $1, 'chat', $2, $3, $3)`, id, status, created); err != nil {
		t.Fatalf("insert trace %s: %v", id, err)
	}
}

func insertInterruptSpanRow(t *testing.T, r *monitorRepo, traceID, spanID string, startedAtMs, endedAtMs int64) {
	t.Helper()
	if _, err := r.data.rawDB.ExecContext(context.Background(),
		`INSERT INTO monitor_trace_spans (trace_id, span_id, kind, name, started_at, ended_at, status)
		 VALUES ($1, $2, 'llm', $2, $3, $4, 'ok')`, traceID, spanID, startedAtMs, endedAtMs); err != nil {
		t.Fatalf("insert span %s/%s: %v", traceID, spanID, err)
	}
}

func interruptTraceStatus(t *testing.T, r *monitorRepo, id string) string {
	t.Helper()
	var status string
	if err := r.data.rawDB.QueryRowContext(context.Background(),
		`SELECT status FROM monitor_traces WHERE id = $1`, id).Scan(&status); err != nil {
		t.Fatalf("read status %s: %v", id, err)
	}
	return status
}

// A stale-created trace with recent span activity is still alive (long-running
// team runs legitimately exceed the TTL) and must NOT be interrupted; only
// traces with no span activity inside the TTL window are swept.
func TestInterruptStaleTraces_SkipsTraceWithRecentSpanActivity(t *testing.T) {
	r := setupInterruptRepo(t)
	ctx := context.Background()
	now := time.Now()
	staleCreated := now.Add(-time.Hour)
	cutoff := now.Add(-30 * time.Minute)

	// Alive: created 1h ago, but a span ended 5 minutes ago.
	insertInterruptTraceRow(t, r, "tr-alive", "running", staleCreated)
	insertInterruptSpanRow(t, r, "tr-alive", "s1",
		staleCreated.Add(20*time.Minute).UnixMilli(), now.Add(-5*time.Minute).UnixMilli())

	// Dead: created 1h ago, last span ended 50 minutes ago.
	insertInterruptTraceRow(t, r, "tr-dead", "running", staleCreated)
	insertInterruptSpanRow(t, r, "tr-dead", "s1",
		staleCreated.Add(time.Minute).UnixMilli(), staleCreated.Add(10*time.Minute).UnixMilli())

	// Dead: created 1h ago, no spans at all.
	insertInterruptTraceRow(t, r, "tr-nospan", "running", staleCreated)

	// Young: created 10 minutes ago, no spans — not yet past the TTL.
	insertInterruptTraceRow(t, r, "tr-young", "running", now.Add(-10*time.Minute))

	// Terminal: old but already completed — must not be touched.
	insertInterruptTraceRow(t, r, "tr-done", "ok", staleCreated)

	n, err := r.InterruptStaleTraces(ctx, cutoff)
	if err != nil {
		t.Fatalf("InterruptStaleTraces: %v", err)
	}
	if n != 2 {
		t.Errorf("interrupted = %d, want 2 (tr-dead + tr-nospan)", n)
	}
	want := map[string]string{
		"tr-alive":  "running",
		"tr-dead":   "interrupted",
		"tr-nospan": "interrupted",
		"tr-young":  "running",
		"tr-done":   "ok",
	}
	for id, w := range want {
		if got := interruptTraceStatus(t, r, id); got != w {
			t.Errorf("status[%s] = %q, want %q", id, got, w)
		}
	}
}
