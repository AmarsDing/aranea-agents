package main

import (
	"database/sql"
	"fmt"
	"os"

	_ "github.com/lib/pq"
)

func main() {
	dsn := "postgres://postgres:Hangshan%40123@127.0.0.1:5432/aranea?sslmode=disable"
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		fmt.Println("open:", err)
		os.Exit(1)
	}
	defer db.Close()

	q := func(label, query string, args ...any) {
		fmt.Printf("\n=== %s ===\n", label)
		rows, err := db.Query(query, args...)
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

	// 1. session_turns status distribution overall
	q("session_turns status dist", `SELECT status, COUNT(*)::text FROM session_turns GROUP BY status`)

	// 2. interrupted traces joined to session_turns by run_id
	q("interrupted -> session_turns by run_id", `
		SELECT st.status, COUNT(*)::text
		FROM monitor_traces t
		JOIN session_turns st ON st.run_id = t.run_id
		WHERE t.status='interrupted' AND t.deleted_at=''
		GROUP BY st.status`)

	// 3. run_id with no session_turns match
	q("run_id not in session_turns", `
		SELECT COUNT(*)::text FROM monitor_traces t
		WHERE t.status='interrupted' AND t.deleted_at='' AND t.run_id != ''
		AND NOT EXISTS (SELECT 1 FROM session_turns st WHERE st.run_id = t.run_id)`)

	// 4. for traces WITHOUT run_id: match session_turns by session_id + created_at proximity (turn started within trace window)
	q("no-run_id traces matched by session+time", `
		SELECT st.status, COUNT(DISTINCT t.id)::text
		FROM monitor_traces t
		JOIN session_turns st ON st.session_id = t.session_id
		  AND st.started_at >= t.created_at
		  AND st.started_at <= t.created_at::timestamptz + interval '10 minutes'
		WHERE t.status='interrupted' AND t.deleted_at='' AND t.run_id = ''
		GROUP BY st.status`)

	// 5. combined coverage: how many of 599 get a definitive session_turns status
	q("coverage summary", `
		SELECT
		  COUNT(*) FILTER (WHERE t.run_id != '' AND EXISTS (SELECT 1 FROM session_turns st WHERE st.run_id = t.run_id))::text by_run_id,
		  COUNT(*) FILTER (WHERE t.run_id = '' AND EXISTS (
		    SELECT 1 FROM session_turns st WHERE st.session_id = t.session_id
		    AND st.started_at >= t.created_at
		    AND st.started_at <= t.created_at::timestamptz + interval '10 minutes'))::text by_sess_time,
		  COUNT(*)::text total
		FROM monitor_traces t
		WHERE t.status='interrupted' AND t.deleted_at=''`)

	// 6. sample: session_turns rows for a few interrupted traces (check duration/tokens usable)
	q("sample session_turns join", `
		SELECT t.id trace_id, st.status, st.duration_ms::text, st.total_tokens::text,
		       st.total_cost_micro_usd::text, st.final_provider, st.final_model, st.error_code
		FROM monitor_traces t
		JOIN session_turns st ON st.run_id = t.run_id
		WHERE t.status='interrupted' AND t.deleted_at=''
		ORDER BY t.created_at DESC LIMIT 8`)
}
