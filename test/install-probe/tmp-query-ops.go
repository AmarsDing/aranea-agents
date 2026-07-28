// 统计当前 skill 总数与清单 + 运维相关 agent
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

	// skill 表结构
	fmt.Println("== skill columns ==")
	rows, err := db.Query(`SELECT column_name FROM information_schema.columns WHERE table_name='skill' ORDER BY ordinal_position`)
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

	var total int
	db.QueryRow(`SELECT COUNT(*) FROM skill`).Scan(&total)
	fmt.Printf("\ntotal skills: %d\n", total)

	// 运维相关 agent：name/agent_key/description 含 运维/ops/devops/sre
	fmt.Println("\n== ops-related agents ==")
	rows2, err := db.Query(`SELECT agent_key, name, COALESCE(description,''), kind FROM agent WHERE LOWER(name) LIKE '%运维%' OR LOWER(agent_key) LIKE '%ops%' OR LOWER(agent_key) LIKE '%devops%' OR LOWER(agent_key) LIKE '%sre%' OR LOWER(description) LIKE '%运维%' ORDER BY agent_key`)
	if err != nil {
		fmt.Println("agent query err:", err)
		// 尝试其他表名
		rows2, err = db.Query(`SELECT agent_key, name, COALESCE(description,''), kind FROM agents WHERE LOWER(name) LIKE '%运维%' OR LOWER(agent_key) LIKE '%ops%' ORDER BY agent_key`)
		if err != nil {
			fmt.Println("agents query err:", err)
			return
		}
	}
	defer rows2.Close()
	cnt := 0
	for rows2.Next() {
		var k, n, d, kind string
		rows2.Scan(&k, &n, &d, &kind)
		fmt.Printf("%-40s %-24s kind=%-16s %s\n", k, n, kind, trunc(d, 60))
		cnt++
	}
	if cnt == 0 {
		fmt.Println("(none found by keyword)")
	}
}

func trunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
