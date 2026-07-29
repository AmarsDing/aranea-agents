// 查询运维 agent 的工具配置与运行时设置
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

	// agents 表关键配置列
	fmt.Println("== ops agents config ==")
	rows, err := db.Query(`SELECT agent_key, display_name, COALESCE(tool_profile,''), COALESCE(model,''), enabled FROM agents WHERE agent_key LIKE 'ops_%' ORDER BY agent_key`)
	if err != nil {
		fmt.Println("err1:", err)
		rows, err = db.Query(`SELECT agent_key, display_name, kind FROM agents WHERE agent_key LIKE 'ops_%' ORDER BY agent_key`)
		if err != nil {
			fmt.Println("err2:", err)
			return
		}
	}
	defer rows.Close()
	for rows.Next() {
		var k, n, tp, m string
		var en bool
		if err := rows.Scan(&k, &n, &tp, &m, &en); err != nil {
			fmt.Println("scan err (fallback):", err)
			break
		}
		fmt.Printf("%-28s %-20s profile=%-14s model=%-28s enabled=%v\n", k, n, tp, m, en)
	}

	// agent_runtime_settings 表？
	fmt.Println("\n== runtime settings table? ==")
	var tbl string
	err = db.QueryRow(`SELECT table_name FROM information_schema.tables WHERE table_name LIKE '%runtime_setting%' LIMIT 1`).Scan(&tbl)
	if err != nil {
		fmt.Println("no runtime settings table:", err)
	} else {
		fmt.Println("found:", tbl)
		rows2, err := db.Query(fmt.Sprintf(`SELECT column_name FROM information_schema.columns WHERE table_name='%s' ORDER BY ordinal_position`, tbl))
		if err == nil {
			for rows2.Next() {
				var c string
				rows2.Scan(&c)
				fmt.Print(c, " ")
			}
			rows2.Close()
			fmt.Println()
		}
	}

	// skill 与 agent 绑定表？
	fmt.Println("\n== agent-skill binding tables ==")
	rows3, err := db.Query(`SELECT table_name FROM information_schema.tables WHERE table_name LIKE '%skill%'`)
	if err == nil {
		for rows3.Next() {
			var t string
			rows3.Scan(&t)
			fmt.Println(t)
		}
		rows3.Close()
	}
}
