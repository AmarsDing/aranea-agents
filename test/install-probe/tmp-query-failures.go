// 诊断批次1缺失 skill 的失败原因：查成员会话事件中与缺失 subpath 相关的记录
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

	sessions := []string{
		"44880f5a-a812-442c-9c22-c6235aadcb73",
		"3f786380-88d6-4b8b-935b-28dfd0906ee6",
		"5031ff01-07fa-4e9b-a59d-aa6a3a60439c",
		"5a033a07-1a9a-4e79-a68c-e6a4b31ed2a6",
	}
	targets := []string{"brand-guidelines", "doc-coauthoring", "frontend-design", "pptx"}

	for _, sid := range sessions {
		rows, err := db.Query(`SELECT event FROM trpc_session_events WHERE session_id=$1 ORDER BY created_at ASC`, sid)
		if err != nil {
			fmt.Println("query err:", err)
			continue
		}
		for rows.Next() {
			var ev string
			rows.Scan(&ev)
			for _, t := range targets {
				if strings.Contains(ev, t) {
					// 截取 target 前后各 500 字符
					idx := strings.Index(ev, t)
					start := idx - 400
					if start < 0 {
						start = 0
					}
					end := idx + 900
					if end > len(ev) {
						end = len(ev)
					}
					fmt.Printf("===== session %s ... [%s] =====\n%s\n\n", sid[:8], t, ev[start:end])
					break
				}
			}
		}
		rows.Close()
	}
}
