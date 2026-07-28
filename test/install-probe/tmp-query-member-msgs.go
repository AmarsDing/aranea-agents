// 查询成员会话的 LLM 响应事件，诊断是否产生 tool_call
package main

import (
	"database/sql"
	"fmt"

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
		"3c730e21-c061-44d6-a56f-99fc8c48f8c9",
		"bdeda93d-f5e4-441c-8dd1-c747cd702cca",
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
			// 只打印含 tool_call / response 完成事件的关键片段
			if len(ev) > 1500 {
				ev = ev[:1500]
			}
			fmt.Printf("--- event %d ---\n%s\n", i, ev)
			i++
			if i > 30 {
				fmt.Println("...(truncated)")
				break
			}
		}
		rows.Close()
	}
}
