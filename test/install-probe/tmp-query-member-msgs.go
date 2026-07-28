// 查询成员会话的 LLM 响应事件，诊断是否产生 tool_call
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

	// 探查 trpc_session_events 结构
	cols, err := db.Query(`SELECT column_name, data_type FROM information_schema.columns WHERE table_name='trpc_session_events' ORDER BY ordinal_position`)
	if err != nil {
		panic(err)
	}
	fmt.Println("== trpc_session_events columns ==")
	for cols.Next() {
		var n, t string
		cols.Scan(&n, &t)
		fmt.Printf("  %s %s\n", n, t)
	}
	cols.Close()

	sessions := []string{
		"21a5b8a4-58a5-4701-b6fb-8baaef43fb55",
		"c9a1baaf-8dcc-4bec-9e25-8bcf8fba6774",
	}
	for _, sid := range sessions {
		fmt.Printf("\n===== session %s =====\n", sid)
		rows, err := db.Query(`SELECT event FROM trpc_session_events WHERE session_id=$1 ORDER BY created_at ASC`, sid)
		if err != nil {
			fmt.Println("query err:", err)
			continue
		}
		i := 0
		for rows.Next() {
			var ev string
			rows.Scan(&ev)
			// 只打印含 tool_call / tool response / error 的事件
			if !strings.Contains(ev, "tool_call") && !strings.Contains(ev, "cli_admin") && !strings.Contains(ev, "tool_response") && !strings.Contains(ev, "error") && !strings.Contains(ev, "set_deliverable") {
				continue
			}
			if len(ev) > 2500 {
				ev = ev[:2500]
			}
			fmt.Printf("--- event %d ---\n%s\n", i, ev)
			i++
			if i > 20 {
				fmt.Println("...(truncated)")
				break
			}
		}
		rows.Close()
	}
}
