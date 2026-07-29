package main

import (
	"database/sql"
	"fmt"
	"os"

	_ "github.com/lib/pq"
)

// Quantify the time delta between an interrupted trace's created_at and the
// nearest session_turns.started_at in the same session, to pick a safe
// matching window for step 4 of the backfill migration.
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

	// delta (seconds) between trace created_at and nearest session_turn started_at
	// for the rows step 4 would confirm.
	q("delta dist (sec) for nearest completed/failed turn", `
		SELECT bucket, COUNT(*)::text FROM (
		  SELECT CASE
		           WHEN delta < 5 THEN 'a_<5s'
		           WHEN delta < 30 THEN 'b_<30s'
		           WHEN delta < 120 THEN 'c_<2m'
		           WHEN delta < 300 THEN 'd_<5m'
		           ELSE 'e_>=5m' END AS bucket
		  FROM (
		    SELECT t.id,
		           ABS(EXTRACT(EPOCH FROM (
		             (SELECT st.started_at::timestamptz FROM session_turns st
		              WHERE st.session_id = t.session_id AND st.started_at <> ''
		                AND st.status IN ('completed','failed')
		                AND st.started_at::timestamptz >= t.created_at::timestamptz - interval '1 minute'
		                AND st.started_at::timestamptz <= t.created_at::timestamptz + interval '15 minutes'
		              ORDER BY st.started_at ASC LIMIT 1) - t.created_at::timestamptz))) AS delta
		    FROM monitor_traces t
		    WHERE t.status='interrupted' AND t.deleted_at='' AND t.session_id <> ''
		  ) d WHERE delta IS NOT NULL
		) b GROUP BY bucket ORDER BY bucket`)

	// with a <=2m closest-match window, how many get confirmed?
	q("confirmed with <=2m window (earliest after -1m)", `
		SELECT COUNT(*)::text FROM monitor_traces t
		WHERE t.status='interrupted' AND t.deleted_at='' AND t.session_id <> ''
		AND EXISTS (
		  SELECT 1 FROM session_turns st
		  WHERE st.session_id = t.session_id AND st.started_at <> ''
		    AND st.status IN ('completed','failed')
		    AND st.started_at::timestamptz >= t.created_at::timestamptz - interval '1 minute'
		    AND st.started_at::timestamptz <= t.created_at::timestamptz + interval '2 minutes'
		)`)

	// multiple candidate turns inside 15m window (collision risk)
	q("traces with >1 candidate turn in 15m window", `
		SELECT COUNT(*)::text FROM (
		  SELECT t.id, COUNT(st.id) AS c
		  FROM monitor_traces t
		  JOIN session_turns st ON st.session_id = t.session_id
		    AND st.started_at <> ''
		    AND st.status IN ('completed','failed')
		    AND st.started_at::timestamptz >= t.created_at::timestamptz - interval '1 minute'
		    AND st.started_at::timestamptz <= t.created_at::timestamptz + interval '15 minutes'
		  WHERE t.status='interrupted' AND t.deleted_at='' AND t.session_id <> ''
		  GROUP BY t.id
		) x WHERE c > 1`)
}
