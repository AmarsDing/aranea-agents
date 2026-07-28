// 查询最近一次 missing4 批次的执行情况
//go:build ignore

package main

import (
	"database/sql"
	"fmt"
	"strings"

	_ "github.com/lib/pq"
)

func main() {
	dsn := "postgres://postgres:Hangshan%40123@127.0.0.1:5432/aranea?sslmode=disable"
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		panic(err)
	}
	defer db.Close()

	// 1. team_runs_v2 表结构
	fmt.Println("== team_runs_v2 columns ==")
	rows, err := db.Query(`SELECT column_name FROM information_schema.columns WHERE table_name='team_runs_v2' ORDER BY ordinal_position`)
	if err != nil {
		panic(err)
	}
	for rows.Next() {
		var c string
		rows.Scan(&c)
		fmt.Print(c, " ")
	}
	rows.Close()
	fmt.Println()

	// 2. 最近的 team runs
	fmt.Println("\n== recent team runs (last 30 min) ==")
	rows2, err := db.Query(`SELECT id, status, COALESCE(error_message,''), created_at FROM team_runs_v2 WHERE created_at::timestamptz > NOW() - INTERVAL '30 minutes' ORDER BY created_at DESC LIMIT 5`)
	if err != nil {
		fmt.Println("err:", err)
	} else {
		for rows2.Next() {
			var id, st, e, ca string
			rows2.Scan(&id, &st, &e, &ca)
			fmt.Printf("run %s status=%s created=%s err=%s\n", id, st, ca, trunc(e, 300))
		}
		rows2.Close()
	}

	// 3. 成员会话中的工具调用/错误（jsonb 需 ::text）
	fmt.Println("\n== recent events mentioning install tool or error ==")
	rows3, err := db.Query(`SELECT session_id, event::text FROM trpc_session_events WHERE created_at::timestamptz > NOW() - INTERVAL '30 minutes' AND (event::text LIKE '%cli_admin_skill_install%' OR event::text LIKE '%skill_install%') ORDER BY created_at ASC LIMIT 10`)
	if err != nil {
		fmt.Println("err:", err)
	} else {
		cnt := 0
		for rows3.Next() {
			var sid, ev string
			rows3.Scan(&sid, &ev)
			idx := strings.Index(ev, "skill_install")
			start := idx - 200
			if start < 0 {
				start = 0
			}
			end := idx + 1200
			if end > len(ev) {
				end = len(ev)
			}
			fmt.Printf("--- session %s ---\n%s\n\n", sid[:8], ev[start:end])
			cnt++
		}
		rows3.Close()
		if cnt == 0 {
			fmt.Println("(no install tool events)")
		}
	}
}

func trunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
