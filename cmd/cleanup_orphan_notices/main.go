package main

import (
	"database/sql"
	"fmt"
	"os"

	_ "github.com/lib/pq"
)

func main() {
	dsn := "postgres://postgres:Hangshan%40123@127.0.0.1:5432/aranea?sslmode=disable"
	if v := os.Getenv("PG_DSN"); v != "" {
		dsn = v
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		fmt.Fprintln(os.Stderr, "open:", err)
		os.Exit(1)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		fmt.Fprintln(os.Stderr, "ping:", err)
		os.Exit(1)
	}

	// 1. Preview: count + list the orphan notices to be deleted.
	var total int
	if err := db.QueryRow(`
SELECT count(*) FROM steps_v2
WHERE kind = 'notice' AND turn_id = '' AND task_id = '' AND author_agent_key = 'pre-planning-gate'`).Scan(&total); err != nil {
		fmt.Fprintln(os.Stderr, "count:", err)
		os.Exit(1)
	}
	fmt.Printf("Found %d orphan pre-planning-gate notice(s) to delete.\n", total)
	if total == 0 {
		fmt.Println("Nothing to do.")
		return
	}

	rows, err := db.Query(`
SELECT id, session_id, left(content, 60), to_char(started_at, 'YYYY-MM-DD HH24:MI:SS')
FROM steps_v2
WHERE kind = 'notice' AND turn_id = '' AND task_id = '' AND author_agent_key = 'pre-planning-gate'
ORDER BY started_at`)
	if err != nil {
		fmt.Fprintln(os.Stderr, "query:", err)
		os.Exit(1)
	}
	for rows.Next() {
		var id, sessionID, content, ts string
		if err := rows.Scan(&id, &sessionID, &content, &ts); err != nil {
			fmt.Fprintln(os.Stderr, "scan:", err)
			os.Exit(1)
		}
		fmt.Printf("  %s  %s  %s  %q\n", ts, id, sessionID, content)
	}
	rows.Close()

	// 2. Delete.
	res, err := db.Exec(`
DELETE FROM steps_v2
WHERE kind = 'notice' AND turn_id = '' AND task_id = '' AND author_agent_key = 'pre-planning-gate'`)
	if err != nil {
		fmt.Fprintln(os.Stderr, "delete:", err)
		os.Exit(1)
	}
	affected, _ := res.RowsAffected()
	fmt.Printf("Deleted %d orphan notice(s).\n", affected)

	// 3. Verify.
	var remaining int
	if err := db.QueryRow(`
SELECT count(*) FROM steps_v2
WHERE kind = 'notice' AND turn_id = '' AND task_id = '' AND author_agent_key = 'pre-planning-gate'`).Scan(&remaining); err != nil {
		fmt.Fprintln(os.Stderr, "verify:", err)
		os.Exit(1)
	}
	fmt.Printf("Remaining orphan notices: %d\n", remaining)
}
