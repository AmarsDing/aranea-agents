// 一次性排查：steps_v2 表中精灵助手 session 的 task/reply 分布（2026-07-29）
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

	queries := []struct{ title, q string }{
		{"== steps_v2 按 kind 全库分布 ==", `SELECT kind, count(*) FROM steps_v2 GROUP BY 1 ORDER BY 2 DESC LIMIT 20`},
		{"== ec86e351 steps 按 kind ==", `SELECT kind, count(*) FROM steps_v2 WHERE session_id='ec86e351-88fc-4ffd-88d8-0ffce1e8af53' GROUP BY 1`},
		{"== ec86e351 task/reply 样例 ==", `SELECT kind, status, left(COALESCE(content,''),100) c, started_at FROM steps_v2 WHERE session_id='ec86e351-88fc-4ffd-88d8-0ffce1e8af53' AND kind IN ('task','reply') ORDER BY started_at ASC LIMIT 15`},
		{"== steps_v2 中含 task 的 session 排行 ==", `SELECT session_id, count(*) FILTER (WHERE kind='task') tasks, count(*) FILTER (WHERE kind='reply') replies, max(started_at) latest FROM steps_v2 GROUP BY 1 HAVING count(*) FILTER (WHERE kind='task') > 0 ORDER BY latest DESC LIMIT 10`},
		{"== 精灵助手 73 个 session 的 task 覆盖 ==", `SELECT count(DISTINCT s.id) spirit_sessions, count(DISTINCT st.session_id) with_tasks FROM sessions s LEFT JOIN steps_v2 st ON st.session_id=s.id AND st.kind='task' WHERE s.agent_id='agent___spirit__'`},
	}
	for _, it := range queries {
		fmt.Println("\n" + it.title)
		rows, err := db.Query(it.q)
		if err != nil {
			fmt.Println("  ERR:", err)
			continue
		}
		cols, _ := rows.Columns()
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		n := 0
		for rows.Next() {
			if err := rows.Scan(ptrs...); err != nil {
				fmt.Println("  scan ERR:", err)
				break
			}
			fmt.Print("  ")
			for i, c := range cols {
				fmt.Printf("%s=%v  ", c, vals[i])
			}
			fmt.Println()
			n++
		}
		if n == 0 {
			fmt.Println("  (0 rows)")
		}
		rows.Close()
	}
}
