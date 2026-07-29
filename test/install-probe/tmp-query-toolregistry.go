// 查所有 tool 相关表与关键工具状态
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

	rows, _ := db.Query(`SELECT table_name FROM information_schema.tables WHERE table_name LIKE '%tool%' ORDER BY table_name`)
	var tbls []string
	for rows.Next() {
		var t string
		rows.Scan(&t)
		tbls = append(tbls, t)
	}
	rows.Close()
	fmt.Println("tool tables:", tbls)

	for _, t := range tbls {
		fmt.Printf("\n== %s ==\n", t)
		r2, err := db.Query(fmt.Sprintf(`SELECT column_name FROM information_schema.columns WHERE table_name='%s' ORDER BY ordinal_position`, t))
		if err != nil {
			continue
		}
		var cols []string
		for r2.Next() {
			var c string
			r2.Scan(&c)
			cols = append(cols, c)
		}
		r2.Close()
		fmt.Println("cols:", cols)

		// 找 key/name/enabled 列
		keyCol, nameCol, enCol := "", "", ""
		for _, c := range cols {
			switch c {
			case "key", "tool_key", "tool_key_id":
				if keyCol == "" {
					keyCol = c
				}
			case "name", "display_name":
				if nameCol == "" {
					nameCol = c
				}
			case "enabled", "is_enabled":
				enCol = c
			}
		}
		if keyCol == "" {
			continue
		}
		sel := keyCol
		if nameCol != "" {
			sel += "," + nameCol
		} else {
			sel += ",'' "
		}
		if enCol != "" {
			sel += "," + enCol
		} else {
			sel += ",true"
		}
		q := fmt.Sprintf(`SELECT %s FROM %s WHERE %s IN ('web_fetch','skill_run','skill_exec','skill_load','skill_search','shell_exec','exec_command','memory_search','knowledge_search','cli_admin_skill_install_from_url','gemini_web_fetch') ORDER BY 1`, sel, t, keyCol)
		r3, err := db.Query(q)
		if err != nil {
			fmt.Println(" q err:", err)
			continue
		}
		for r3.Next() {
			var k, n string
			var en bool
			r3.Scan(&k, &n, &en)
			fmt.Printf("  %-36s %-28s enabled=%v\n", k, n, en)
		}
		r3.Close()
	}
}
