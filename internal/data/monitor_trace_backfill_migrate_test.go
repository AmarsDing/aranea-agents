package data

import (
	"context"
	"testing"

	"aranea-agents/internal/data/testhelper"
	"aranea-agents/pkg/loggateway"
)

// TestRunMonitorTraceInterruptedBackfillMigration verifies the one-time
// reclassification of historical "interrupted" monitor traces. Root cause of
// the bad data: RecordRunnerCompletion had no production callers for weeks,
// so the sweeper marked every trace "interrupted" after 30 minutes. Span
// evidence (all 599 rows have spans) plus session_turns ground truth allow
// truthful reclassification.
func TestRunMonitorTraceInterruptedBackfillMigration(t *testing.T) {
	ctx := context.Background()
	lg := loggateway.NewNoop()

	client, _ := testhelper.SetupTestPG(t)
	d := newDataFromClient(client, lg)

	// Minimal table set (raw-SQL tables, no Ent schema).
	for _, ddl := range []string{
		`CREATE TABLE IF NOT EXISTS monitor_traces (
			id TEXT PRIMARY KEY,
			trace_key TEXT NOT NULL DEFAULT '',
			name TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'ok',
			session_id TEXT NOT NULL DEFAULT '',
			run_id TEXT NOT NULL DEFAULT '',
			agent_id TEXT NOT NULL DEFAULT '',
			provider TEXT NOT NULL DEFAULT '',
			model TEXT NOT NULL DEFAULT '',
			duration_ms INTEGER NOT NULL DEFAULT 0,
			span_count INTEGER NOT NULL DEFAULT 0,
			error_count INTEGER NOT NULL DEFAULT 0,
			total_tokens INTEGER NOT NULL DEFAULT 0,
			total_cost_usd REAL NOT NULL DEFAULT 0.0,
			metadata_json TEXT NOT NULL DEFAULT '{}',
			created_at TEXT NOT NULL DEFAULT '',
			updated_at TEXT NOT NULL DEFAULT '',
			deleted_at TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS monitor_trace_spans (
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
		)`,
		`CREATE TABLE IF NOT EXISTS session_turns (
			id TEXT PRIMARY KEY,
			session_id TEXT NOT NULL DEFAULT '',
			run_id TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT '',
			started_at TEXT NOT NULL DEFAULT '',
			ended_at TEXT NOT NULL DEFAULT '',
			duration_ms BIGINT NOT NULL DEFAULT 0,
			total_tokens BIGINT NOT NULL DEFAULT 0,
			final_provider TEXT NOT NULL DEFAULT '',
			final_model TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL DEFAULT '',
			updated_at TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS model_token_usage_events (
			id TEXT PRIMARY KEY,
			occurred_at TEXT NOT NULL,
			session_id TEXT NOT NULL DEFAULT '',
			provider_code TEXT NOT NULL DEFAULT '',
			model_api_id TEXT NOT NULL DEFAULT '',
			total_tokens INTEGER NOT NULL DEFAULT 0,
			total_cost_micro_usd INTEGER NOT NULL DEFAULT 0,
			metadata_json TEXT NOT NULL DEFAULT '{}',
			created_at TEXT NOT NULL DEFAULT ''
		)`,
	} {
		if _, err := client.ExecContext(ctx, ddl); err != nil {
			t.Fatalf("create table: %v\n%s", err, ddl)
		}
	}

	// --- Fixtures ---
	// tr_ok_terminal: completed chain (last span chat.turn.execute) → ok
	// tr_err_span: one error span → error
	// tr_sess_confirm: mid-chain last span, session_turns confirms completed → ok (+turn tokens/provider/model)
	// tr_sess_failed: mid-chain, session_turns says failed → error
	// tr_stays: mid-chain, no corroboration → stays interrupted (span metrics still backfilled)
	// tr_usage: ok via terminal span + usage events by trace_id → tokens/cost/provider/model from usage
	// tr_already_ok: status ok → untouched
	// tr_deleted: soft-deleted interrupted → untouched
	traces := []struct{ id, status, sessionID, runID, createdAt, deletedAt string }{
		{"tr_ok_terminal", "interrupted", "s1", "r1", "2026-07-20T10:00:00Z", ""},
		{"tr_err_span", "interrupted", "s2", "r2", "2026-07-20T11:00:00Z", ""},
		{"tr_sess_confirm", "interrupted", "s3", "", "2026-07-20T12:00:00Z", ""},
		{"tr_sess_failed", "interrupted", "s4", "", "2026-07-20T13:00:00Z", ""},
		{"tr_stays", "interrupted", "s5", "", "2026-07-20T14:00:00Z", ""},
		{"tr_usage", "interrupted", "s6", "r6", "2026-07-20T15:00:00Z", ""},
		{"tr_already_ok", "ok", "s7", "r7", "2026-07-20T16:00:00Z", ""},
		{"tr_deleted", "interrupted", "s8", "", "2026-07-20T17:00:00Z", "2026-07-21T00:00:00Z"},
	}
	for _, tr := range traces {
		if _, err := client.ExecContext(ctx,
			`INSERT INTO monitor_traces (id, status, session_id, run_id, created_at, updated_at, deleted_at)
			 VALUES ($1, $2, $3, $4, $5, $5, $6)`,
			tr.id, tr.status, tr.sessionID, tr.runID, tr.createdAt, tr.deletedAt); err != nil {
			t.Fatalf("insert trace %s: %v", tr.id, err)
		}
	}

	type span struct {
		traceID, spanID, name, status string
		start, end                    int64
	}
	base := int64(1785000000000) // ms
	spans := []span{
		// completed chain, ends with chat.turn.execute (3000ms range)
		{"tr_ok_terminal", "sp1", "chat.receive", "ok", base, base},
		{"tr_ok_terminal", "sp2", "chat.llm.invoke", "ok", base + 100, base + 2000},
		{"tr_ok_terminal", "sp3", "chat.turn.execute", "ok", base, base + 3000},
		// error chain
		{"tr_err_span", "sp1", "chat.receive", "ok", base, base},
		{"tr_err_span", "sp2", "chat.agent_hydrate", "error", base + 10, base + 50},
		// mid-chain (turn.enter), confirmed by session_turns
		{"tr_sess_confirm", "sp1", "chat.receive", "ok", base, base},
		{"tr_sess_confirm", "sp2", "chat.turn.enter", "ok", base + 5, base + 8},
		// mid-chain (llm.invoke), session_turns says failed
		{"tr_sess_failed", "sp1", "chat.receive", "ok", base, base},
		{"tr_sess_failed", "sp2", "chat.llm.invoke", "ok", base + 10, base + 900},
		// mid-chain, nothing else → stays interrupted
		{"tr_stays", "sp1", "chat.receive", "ok", base, base},
		{"tr_stays", "sp2", "chat.provider_resolve", "ok", base + 4, base + 6},
		// terminal span + usage events
		{"tr_usage", "sp1", "chat.receive", "ok", base, base},
		{"tr_usage", "sp2", "chat.assistant_msg_persist", "ok", base + 10, base + 1500},
		// already-ok trace: spans exist but row must stay untouched
		{"tr_already_ok", "sp1", "chat.receive", "ok", base, base},
		// deleted trace: terminal span but soft-deleted
		{"tr_deleted", "sp1", "chat.turn.execute", "ok", base, base + 100},
	}
	for _, sp := range spans {
		if _, err := client.ExecContext(ctx,
			`INSERT INTO monitor_trace_spans (trace_id, span_id, kind, name, started_at, ended_at, status)
			 VALUES ($1, $2, 'step', $3, $4, $5, $6)`,
			sp.traceID, sp.spanID, sp.name, sp.start, sp.end, sp.status); err != nil {
			t.Fatalf("insert span %s/%s: %v", sp.traceID, sp.spanID, err)
		}
	}

	// session_turns ground truth (started_at varchar RFC3339).
	turns := []struct {
		id, sessionID, status, startedAt, provider, model string
		tokens                                            int64
	}{
		{"st1", "s3", "completed", "2026-07-20T12:00:01Z", "deepseek", "deepseek-chat", 1234},
		{"st2", "s4", "failed", "2026-07-20T13:00:02Z", "openai", "gpt-5", 77},
	}
	for _, st := range turns {
		if _, err := client.ExecContext(ctx,
			`INSERT INTO session_turns (id, session_id, status, started_at, total_tokens, final_provider, final_model)
			 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			st.id, st.sessionID, st.status, st.startedAt, st.tokens, st.provider, st.model); err != nil {
			t.Fatalf("insert session_turn %s: %v", st.id, err)
		}
	}

	// usage events carrying trace_id metadata for tr_usage.
	usage := []struct {
		id, traceID, occurredAt, provider, model string
		tokens, costMicro                        int64
	}{
		{"u1", "tr_usage", "2026-07-20T15:00:01Z", "deepseek", "deepseek-v4", 1000, 500},
		{"u2", "tr_usage", "2026-07-20T15:00:02Z", "deepseek", "deepseek-v4", 2000, 1500},
	}
	for _, u := range usage {
		if _, err := client.ExecContext(ctx,
			`INSERT INTO model_token_usage_events (id, occurred_at, session_id, provider_code, model_api_id, total_tokens, total_cost_micro_usd, metadata_json)
			 VALUES ($1, $2, 's6', $3, $4, $5, $6, json_build_object('trace_id', $7::text)::text)`,
			u.id, u.occurredAt, u.provider, u.model, u.tokens, u.costMicro, u.traceID); err != nil {
			t.Fatalf("insert usage %s: %v", u.id, err)
		}
	}

	// --- Run migration (twice for idempotency) ---
	if err := RunMonitorTraceInterruptedBackfillMigration(ctx, client, d.Dialect(), lg); err != nil {
		t.Fatalf("migration: %v", err)
	}
	if err := RunMonitorTraceInterruptedBackfillMigration(ctx, client, d.Dialect(), lg); err != nil {
		t.Fatalf("migration second run: %v", err)
	}

	// --- Assertions ---
	type traceRow struct {
		status     string
		durationMs int64
		spanCount  int64
		errorCount int64
		tokens     int64
		cost       float64
		provider   string
		model      string
	}
	get := func(id string) traceRow {
		rows, err := client.QueryContext(ctx,
			`SELECT status, duration_ms, span_count, error_count, total_tokens, total_cost_usd, provider, model
			 FROM monitor_traces WHERE id = $1`, id)
		if err != nil {
			t.Fatalf("query %s: %v", id, err)
		}
		defer rows.Close()
		if !rows.Next() {
			t.Fatalf("no row for %s", id)
		}
		var r traceRow
		if err := rows.Scan(&r.status, &r.durationMs, &r.spanCount, &r.errorCount, &r.tokens, &r.cost, &r.provider, &r.model); err != nil {
			t.Fatalf("scan %s: %v", id, err)
		}
		return r
	}

	// 1. terminal-span chain → ok, span metrics from span range.
	r := get("tr_ok_terminal")
	if r.status != "ok" {
		t.Errorf("tr_ok_terminal status = %q, want ok", r.status)
	}
	if r.durationMs != 3000 || r.spanCount != 3 || r.errorCount != 0 {
		t.Errorf("tr_ok_terminal metrics = %+v, want dur=3000 spans=3 errs=0", r)
	}

	// 2. error span → error.
	r = get("tr_err_span")
	if r.status != "error" {
		t.Errorf("tr_err_span status = %q, want error", r.status)
	}
	if r.errorCount != 1 {
		t.Errorf("tr_err_span errorCount = %d, want 1", r.errorCount)
	}

	// 3. session_turns confirms completed → ok + turn tokens/provider/model.
	r = get("tr_sess_confirm")
	if r.status != "ok" {
		t.Errorf("tr_sess_confirm status = %q, want ok", r.status)
	}
	if r.tokens != 1234 || r.provider != "deepseek" || r.model != "deepseek-chat" {
		t.Errorf("tr_sess_confirm turn fields = %+v, want tokens=1234 deepseek/deepseek-chat", r)
	}

	// 4. session_turns failed → error.
	r = get("tr_sess_failed")
	if r.status != "error" {
		t.Errorf("tr_sess_failed status = %q, want error", r.status)
	}

	// 5. no corroboration → stays interrupted, span metrics backfilled.
	r = get("tr_stays")
	if r.status != "interrupted" {
		t.Errorf("tr_stays status = %q, want interrupted", r.status)
	}
	if r.spanCount != 2 || r.durationMs != 6 {
		t.Errorf("tr_stays metrics = %+v, want spans=2 dur=6", r)
	}

	// 6. usage events backfill tokens/cost/provider/model.
	r = get("tr_usage")
	if r.status != "ok" {
		t.Errorf("tr_usage status = %q, want ok", r.status)
	}
	if r.tokens != 3000 {
		t.Errorf("tr_usage tokens = %d, want 3000", r.tokens)
	}
	if r.cost < 0.0019 || r.cost > 0.0021 {
		t.Errorf("tr_usage cost = %v, want ~0.002", r.cost)
	}
	if r.provider != "deepseek" || r.model != "deepseek-v4" {
		t.Errorf("tr_usage provider/model = %q/%q, want deepseek/deepseek-v4", r.provider, r.model)
	}

	// 7. already-ok row untouched (duration stays 0, span_count stays 0).
	r = get("tr_already_ok")
	if r.status != "ok" || r.durationMs != 0 || r.spanCount != 0 {
		t.Errorf("tr_already_ok = %+v, want untouched", r)
	}

	// 8. soft-deleted row untouched.
	r = get("tr_deleted")
	if r.status != "interrupted" {
		t.Errorf("tr_deleted status = %q, want interrupted (untouched)", r.status)
	}
}
