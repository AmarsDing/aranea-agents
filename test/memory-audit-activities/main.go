// 一次性排查：activities 表结构与 session ec86e351 实际内容（2026-07-29）
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
		{"== activities 表列 ==", `SELECT column_name, data_type FROM information_schema.columns WHERE table_name='activities' ORDER BY ordinal_position`},
		{"== activities 总量/按kind ==", `SELECT kind, count(*) FROM activities GROUP BY 1 ORDER BY 2 DESC LIMIT 20`},
		{"== ec86e351 全 kind 分布 ==", `SELECT kind, count(*) FROM activities WHERE session_id='ec86e351-88fc-4ffd-88d8-0ffce1e8af53' GROUP BY 1`},
		{"== ec86e351 样例 ==", `SELECT kind, status, left(COALESCE(content,''),80) FROM activities WHERE session_id='ec86e351-88fc-4ffd-88d8-0ffce1e8af53' ORDER BY timestamp ASC LIMIT 20`},
		{"== 全库含 task/reply 的 session 分布 ==", `SELECT session_id, count(*) FILTER (WHERE kind='task') tasks, count(*) FILTER (WHERE kind='reply') replies FROM activities GROUP BY 1 HAVING count(*) FILTER (WHERE kind='task') > 0 ORDER BY tasks DESC LIMIT 10`},
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
