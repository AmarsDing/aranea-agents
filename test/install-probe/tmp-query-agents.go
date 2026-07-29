// 查询 agents 表结构与运维相关 agent
//go:build ignore

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

	fmt.Println("== agents columns ==")
	rows, err := db.Query(`SELECT column_name FROM information_schema.columns WHERE table_name='agents' ORDER BY ordinal_position`)
	if err != nil {
		panic(err)
	}
	var cols []string
	for rows.Next() {
		var c string
		rows.Scan(&c)
		cols = append(cols, c)
		fmt.Print(c, " ")
	}
	rows.Close()
	fmt.Println()

	// 找名称列
	nameCol := "agent_key"
	for _, c := range cols {
		if c == "display_name" || c == "title" || c == "label" {
			nameCol = c
			break
		}
	}
	fmt.Println("\nusing name column:", nameCol)

	// 全量列出 agent_key + 名称 + 岗位
	q := fmt.Sprintf(`SELECT agent_key, COALESCE(%s,''), kind, COALESCE(agent_variant,'') FROM agents ORDER BY agent_key`, nameCol)
	rows2, err := db.Query(q)
	if err != nil {
		fmt.Println("err:", err)
		return
	}
	defer rows2.Close()
	for rows2.Next() {
		var k, n, kind, v string
		rows2.Scan(&k, &n, &kind, &v)
		fmt.Printf("%-42s %-22s kind=%-16s variant=%s\n", k, trunc(n, 22), kind, v)
	}
}

func trunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
