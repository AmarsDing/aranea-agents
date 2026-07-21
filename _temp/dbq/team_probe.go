package main

import (
	"database/sql"
	"fmt"
	"os"

	_ "github.com/lib/pq"
)

// P0-3/P0-4 evidence probe: team state link.
// Q1: team_runs_v2 rows stuck in 'running' (terminal state never persisted)
// Q2: teams table status distribution per recent spirit session
// Q3: cross-check teams vs team_runs_v2 status consistency
// Q4: team_stages_v2 status distribution
func main() {
	dsn := "postgres://postgres:Hangshan%40123@127.0.0.1:5432/aranea?sslmode=disable"
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		fmt.Println("open err:", err)
		os.Exit(1)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		fmt.Println("ping err:", err)
		os.Exit(1)
	}

	run := func(label, q string, args ...any) {
		rows, err := db.Query(q, args...)
		if err != nil {
			fmt.Printf("%s: QUERY ERROR: %v\n", label, err)
			return
		}
		defer rows.Close()
		cols, _ := rows.Columns()
		fmt.Printf("== %s ==\n", label)
		n := 0
		for rows.Next() {
			vals := make([]any, len(cols))
			ptrs := make([]any, len(cols))
			for i := range vals {
				ptrs[i] = &vals[i]
			}
			if err := rows.Scan(ptrs...); err != nil {
				fmt.Println("  scan err:", err)
				return
			}
			fmt.Print("  ")
			for i, c := range cols {
				v := vals[i]
				if b, ok := v.([]byte); ok {
					v = string(b)
				}
				s := fmt.Sprintf("%v", v)
				if len(s) > 60 {
					s = s[:60] + "…"
				}
				fmt.Printf("%s=%s  ", c, s)
			}
			fmt.Println()
			n++
			if n >= 40 {
				fmt.Println("  ... (truncated)")
				break
			}
		}
		if n == 0 {
			fmt.Println("  (0 rows)")
		}
	}

	// Q1: team_runs_v2 stuck running, ordered by started_at
	run("Q1 team_runs_v2 status counts", `
		SELECT status, count(*) FROM team_runs_v2 GROUP BY status ORDER BY 2 DESC`)

	run("Q1b team_runs_v2 running rows (recent 20)", `
		SELECT id, team_stage_id, spirit_session_id, status, started_at, completed_at, version
		FROM team_runs_v2 WHERE status='running' ORDER BY started_at DESC LIMIT 20`)

	// Q2: teams table status distribution
	run("Q2 teams status counts", `
		SELECT status, count(*) FROM teams GROUP BY status ORDER BY 2 DESC`)

	run("Q2b recent teams (latest 15)", `
		SELECT id, spirit_session_id, status, display_name, auto_created, created_at, updated_at
		FROM teams ORDER BY created_at DESC LIMIT 15`)

	// Q3: sessions 12:21 前后的 spirit sessions + team 状态对比
	run("Q3 recent running teams vs their team_runs_v2", `
		SELECT t.id AS team_id, t.status AS team_status, tr.id AS run_id, tr.status AS run_status,
		       tr.started_at, tr.completed_at
		FROM teams t
		LEFT JOIN team_runs_v2 tr ON tr.spirit_session_id = t.spirit_session_id
		WHERE t.status IN ('running','pending') OR tr.status = 'running'
		ORDER BY t.updated_at DESC LIMIT 30`)

	// Q4: team_stages_v2
	run("Q4 team_stages_v2 status counts", `
		SELECT status, count(*) FROM team_stages_v2 GROUP BY status ORDER BY 2 DESC`)

	run("Q4b team_stages_v2 running rows (recent 20)", `
		SELECT id, team_id, session_id, status, stage, started_at
		FROM team_stages_v2 WHERE status IN ('running','pending') ORDER BY started_at DESC LIMIT 20`)

	// Q5: 对照同一 spirit_session 下 teams 与 team_runs_v2 状态不一致
	run("Q5 inconsistent: team terminal but run still running", `
		SELECT t.id AS team_id, t.status AS team_status, t.spirit_session_id,
		       tr.id AS run_id, tr.status AS run_status, tr.started_at
		FROM teams t
		JOIN team_runs_v2 tr ON tr.spirit_session_id = t.spirit_session_id
		WHERE t.status IN ('completed','failed','cancelled') AND tr.status = 'running'
		ORDER BY tr.started_at DESC LIMIT 30`)
}
