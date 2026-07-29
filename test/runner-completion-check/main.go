// One-off debug: verify latest runner.completion rows in monitor_events.
package main

import (
	"database/sql"
	"fmt"

	_ "github.com/lib/pq"
)

func main() {
	db, err := sql.Open("postgres", "postgres://postgres:Hangshan%40123@127.0.0.1:5432/aranea?sslmode=disable")
	if err != nil {
		panic(err)
	}
	defer db.Close()

	var latest sql.NullString
	var total int
	if err := db.QueryRow(`SELECT max(created_at), count(*) FROM monitor_events WHERE event_key='runner.completion' AND deleted_at=''`).Scan(&latest, &total); err != nil {
		panic(err)
	}
	fmt.Printf("runner.completion rows: total=%d latest=%v\n", total, latest.String)

	rows, err := db.Query(`SELECT created_at, status, left(metadata_json, 100) FROM monitor_events WHERE event_key='runner.completion' AND deleted_at='' ORDER BY created_at DESC LIMIT 5`)
	if err != nil {
		panic(err)
	}
	defer rows.Close()
	for rows.Next() {
		var ts, st, meta string
		if err := rows.Scan(&ts, &st, &meta); err != nil {
			panic(err)
		}
		fmt.Printf("  %s status=%s meta=%s\n", ts, st, meta)
	}

	var now string
	_ = db.QueryRow(`SELECT to_char(now(), 'YYYY-MM-DD HH24:MI:SS')`).Scan(&now)
	fmt.Printf("db now: %s\n", now)

	fmt.Println("--- alert rules ---")
	rows2, err := db.Query(`SELECT id, name, metric_key, threshold, window_minutes, enabled, firing_state, COALESCE(last_fired_at,0) FROM monitor_alert_rules`)
	if err != nil {
		panic(err)
	}
	defer rows2.Close()
	for rows2.Next() {
		var id, name, mk, fs string
		var th float64
		var wm int
		var en bool
		var lf int64
		if err := rows2.Scan(&id, &name, &mk, &th, &wm, &en, &fs, &lf); err != nil {
			panic(err)
		}
		fmt.Printf("  rule=%s name=%q metric=%s threshold=%.2f window=%dm enabled=%v state=%s last_fired=%d\n", id, name, mk, th, wm, en, fs, lf)
	}

	fmt.Println("--- self check reports ---")
	var rc int
	var latestReport sql.NullString
	if err := db.QueryRow(`SELECT count(*), max(finished_at) FROM self_check_reports`).Scan(&rc, &latestReport); err != nil {
		fmt.Printf("  self_check_reports query error: %v\n", err)
	} else {
		fmt.Printf("  reports=%d latest=%v\n", rc, latestReport.String)
	}
}
