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

	q := func(label, sql string, args ...any) {
		fmt.Printf("\n=== %s ===\n", label)
		rows, err := db.Query(sql, args...)
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

	q("trace status dist", `SELECT status, COUNT(*) FROM monitor_traces WHERE deleted_at='' GROUP BY status`)
	q("trace cols", `SELECT column_name FROM information_schema.columns WHERE table_name='monitor_traces' ORDER BY ordinal_position`)
	q("span cols", `SELECT column_name FROM information_schema.columns WHERE table_name='monitor_trace_spans' ORDER BY ordinal_position`)
	q("usage cols", `SELECT column_name FROM information_schema.columns WHERE table_name='model_token_usage_events' ORDER BY ordinal_position`)
	q("usage trace_id stats", `SELECT COUNT(*) total, COUNT(*) FILTER (WHERE metadata_json->>'trace_id' IS NOT NULL AND metadata_json->>'trace_id' != '') has_tid FROM model_token_usage_events`)
	q("usage recent", `SELECT metadata_json->>'trace_id' tid, session_id, run_id, status, created_at FROM model_token_usage_events ORDER BY created_at DESC LIMIT 8`)
	q("spans for recent trace", `SELECT span_id, name, status FROM monitor_trace_spans WHERE trace_id=(SELECT trace_key FROM monitor_traces WHERE deleted_at='' ORDER BY created_at DESC LIMIT 1) LIMIT 10`)
}
