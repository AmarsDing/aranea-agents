// 一次性排查：tasks_v2 表用户消息确认（2026-07-29）
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
		{"== tasks_v2 列 ==", `SELECT string_agg(column_name, ', ') FROM information_schema.columns WHERE table_name='tasks_v2'`},
		{"== tasks_v2 总量 ==", `SELECT count(*) FROM tasks_v2`},
		{"== ec86e351 的 tasks ==", `SELECT id, status, left(COALESCE(goal,''),80) FROM tasks_v2 WHERE session_id='ec86e351-88fc-4ffd-88d8-0ffce1e8af53' LIMIT 10`},
		{"== 精灵助手 tasks 覆盖 ==", `SELECT count(DISTINCT s.id) spirit_sessions, count(DISTINCT t.session_id) with_tasks, count(t.id) total_tasks FROM sessions s LEFT JOIN tasks_v2 t ON t.session_id=s.id WHERE s.agent_id='agent___spirit__'`},
		{"== L1 任务 archived_at 状态 ==", `SELECT id, status, COALESCE(archived_at,'<empty>') archived, COALESCE(ended_at,'<empty>') ended FROM memory_l1_tasks WHERE agent_id='agent___spirit__'`},
		{"== ec86e351 用户消息 ==", `SELECT id, status, left(user_message,120) FROM tasks_v2 WHERE session_id='ec86e351-88fc-4ffd-88d8-0ffce1e8af53'`},
		{"== memory_episodes 全表 ==", `SELECT count(*) FROM memory_episodes`},
		{"== memory_episodes 索引 ==", `SELECT indexname, indexdef FROM pg_indexes WHERE tablename='memory_episodes'`},
		{"== schema_migrations 20260802 状态 ==", `SELECT version, name, applied_at FROM schema_migrations WHERE version IN ('20260802','20260902') ORDER BY version`},
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
