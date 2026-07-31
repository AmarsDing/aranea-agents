package main

import (
	"database/sql"
	"fmt"
	"os"
	"time"

	_ "github.com/lib/pq"
)

// Dry-run of RunMonitorTraceInterruptedBackfillMigration against the dev DB:
// applies the exact 4 steps inside a transaction, prints the resulting status
// distribution + samples, then ROLLS BACK.
func main() {
	dsn := "postgres://postgres:Hangshan%40123@127.0.0.1:5432/aranea?sslmode=disable"
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		fmt.Println("open:", err)
		os.Exit(1)
	}
	defer db.Close()

	tx, err := db.Begin()
	if err != nil {
		fmt.Println("begin:", err)
		os.Exit(1)
	}
	defer tx.Rollback() //nolint:errcheck // dry-run by design

	now := time.Now().UTC().Format(time.RFC3339)

	q := func(label, query string, args ...any) {
		fmt.Printf("\n=== %s ===\n", label)
		rows, err := tx.Query(query, args...)
		if err != nil {
			fmt.Println("ERR:", err)
			return
		}
		defer rows.Close()
		cols, _ := rows.Columns()
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		n := 0
		for rows.Next() {
			_ = rows.Scan(ptrs...)
			n++
			fmt.Printf("row%d: ", n)
			for i, c := range cols {
				fmt.Printf("%s=%v | ", c, vals[i])
			}
			fmt.Println()
		}
		if n == 0 {
			fmt.Println("(no rows)")
		}
	}

	q("before", `SELECT status, COUNT(*)::text FROM monitor_traces WHERE deleted_at='' GROUP BY status`)

	// Step 1: span metrics
	res1, err := tx.Exec(`
		UPDATE monitor_traces t
		SET duration_ms = sa.duration_ms, span_count = sa.span_count, error_count = sa.error_count, updated_at = $1
		FROM (
		  SELECT s.trace_id, COUNT(*) AS span_count,
		         COUNT(*) FILTER (WHERE s.status = 'error') AS error_count,
		         GREATEST(MAX(s.ended_at) - MIN(s.started_at), 0) AS duration_ms
		  FROM monitor_trace_spans s GROUP BY s.trace_id
		) sa
		WHERE t.id = sa.trace_id AND t.status = 'interrupted' AND t.deleted_at = ''`, now)
	fmt.Println("step1 span metrics:", rowsAffected(res1), err)

	// Step 2: span reclassify
	res2, err := tx.Exec(`
		UPDATE monitor_traces t
		SET status = CASE WHEN sa.error_count > 0 THEN 'error' ELSE 'ok' END, updated_at = $1
		FROM (
		  SELECT s.trace_id,
		         COUNT(*) FILTER (WHERE s.status = 'error') AS error_count,
		         (ARRAY_AGG(s.name ORDER BY s.ended_at DESC, s.started_at DESC))[1] AS last_span
		  FROM monitor_trace_spans s GROUP BY s.trace_id
		) sa
		WHERE t.id = sa.trace_id AND t.status = 'interrupted' AND t.deleted_at = ''
		  AND (sa.error_count > 0
		       OR sa.last_span IN ('chat.turn.execute', 'chat.assistant_msg_persist', 'team.run.finish'))`, now)
	fmt.Println("step2 span reclassify:", rowsAffected(res2), err)

	// Step 3: usage aggregate（与迁移同一 Dialect.JSONExtract 表达式）
	res3, err := tx.Exec(`
		UPDATE monitor_traces t
		SET total_tokens = ua.tokens, total_cost_usd = ua.cost_usd,
		    provider = CASE WHEN t.provider = '' AND ua.provider != '' THEN ua.provider ELSE t.provider END,
		    model = CASE WHEN t.model = '' AND ua.model != '' THEN ua.model ELSE t.model END,
		    updated_at = $1
		FROM (
		  SELECT COALESCE(NULLIF(u.metadata_json::text, '')::jsonb, '{}'::jsonb) ->> 'trace_id' AS trace_id,
		         SUM(u.total_tokens) AS tokens,
		         SUM(u.total_cost_micro_usd) / 1e6 AS cost_usd,
		         COALESCE((ARRAY_AGG(u.provider_code ORDER BY u.occurred_at DESC) FILTER (WHERE u.provider_code <> ''))[1], '') AS provider,
		         COALESCE((ARRAY_AGG(u.model_api_id ORDER BY u.occurred_at DESC) FILTER (WHERE u.model_api_id <> ''))[1], '') AS model
		  FROM model_token_usage_events u
		  WHERE u.metadata_json <> '' AND u.metadata_json <> '{}'
		  GROUP BY 1
		) ua
		WHERE t.id = ua.trace_id AND t.deleted_at = ''
		  AND t.status IN ('ok', 'error') AND t.total_tokens = 0`, now)
	fmt.Println("step3 usage aggregate:", rowsAffected(res3), err)

	// Step 4: session_turns confirm
	res4, err := tx.Exec(`
		UPDATE monitor_traces t
		SET status = CASE WHEN m.turn_status = 'completed' THEN 'ok' ELSE 'error' END,
		    total_tokens = CASE WHEN t.total_tokens = 0 THEN m.turn_tokens ELSE t.total_tokens END,
		    provider = CASE WHEN t.provider = '' THEN m.final_provider ELSE t.provider END,
		    model = CASE WHEN t.model = '' THEN m.final_model ELSE t.model END,
		    updated_at = $1
		FROM monitor_traces t2
		JOIN LATERAL (
		  SELECT st.status AS turn_status, st.total_tokens AS turn_tokens,
		         st.final_provider, st.final_model
		  FROM session_turns st
		  WHERE st.session_id = t2.session_id
		    AND st.started_at <> ''
		    AND st.started_at::timestamptz >= t2.created_at::timestamptz - interval '1 minute'
		    AND st.started_at::timestamptz <= t2.created_at::timestamptz + interval '2 minutes'
		    AND st.status IN ('completed', 'failed')
		  ORDER BY st.started_at ASC
		  LIMIT 1
		) m ON true
		WHERE t2.id = t.id AND t.status = 'interrupted' AND t.deleted_at = '' AND t.session_id <> ''`, now)
	fmt.Println("step4 session_turns confirm:", rowsAffected(res4), err)

	q("after", `SELECT status, COUNT(*)::text FROM monitor_traces WHERE deleted_at='' GROUP BY status`)

	q("reclassified samples", `
		SELECT id, status, duration_ms::text, span_count::text, error_count::text,
		       total_tokens::text, ROUND(total_cost_usd::numeric, 6)::text cost, provider, model
		FROM monitor_traces WHERE deleted_at='' AND status IN ('ok','error')
		ORDER BY updated_at DESC LIMIT 12`)

	q("remaining interrupted last-span dist", `
		SELECT last.name, COUNT(*)::text
		FROM monitor_traces t
		JOIN LATERAL (
		  SELECT s.name FROM monitor_trace_spans s WHERE s.trace_id = t.id
		  ORDER BY s.ended_at DESC, s.started_at DESC LIMIT 1
		) last ON true
		WHERE t.status='interrupted' AND t.deleted_at=''
		GROUP BY last.name ORDER BY COUNT(*) DESC LIMIT 12`)

	// step4 漏配诊断：剩余 interrupted 中，其 session 是否存在 completed/failed turn？
	// 若大量存在但时间差超出窗口，说明窗口过紧；若根本不存在，则为真实中断。
	q("remaining interrupted: turn existence & nearest delta", `
		SELECT
		  COUNT(*) FILTER (WHERE m.turn_status IS NULL) AS no_turn_at_all,
		  COUNT(*) FILTER (WHERE m.turn_status IS NOT NULL AND m.delta_sec <= 120) AS turn_within_window,
		  COUNT(*) FILTER (WHERE m.turn_status IS NOT NULL AND m.delta_sec > 120) AS turn_outside_window,
		  COALESCE(ROUND(AVG(m.delta_sec) FILTER (WHERE m.turn_status IS NOT NULL))::text, '-') AS avg_delta_sec,
		  COALESCE(MAX(m.delta_sec)::text, '-') AS max_delta_sec
		FROM monitor_traces t
		LEFT JOIN LATERAL (
		  SELECT st.status AS turn_status,
		         ABS(EXTRACT(EPOCH FROM (st.started_at::timestamptz - t.created_at::timestamptz))) AS delta_sec
		  FROM session_turns st
		  WHERE st.session_id = t.session_id AND st.started_at <> ''
		    AND st.status IN ('completed', 'failed')
		  ORDER BY delta_sec ASC LIMIT 1
		) m ON true
		WHERE t.status='interrupted' AND t.deleted_at='' AND t.session_id <> ''`)

	fmt.Println("\n(dry-run: rolling back)")
}

func rowsAffected(res sql.Result) string {
	if res == nil {
		return "(no result)"
	}
	n, _ := res.RowsAffected()
	return fmt.Sprintf("%d rows", n)
}
