// 统计当前 skill 总数与清单
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

	var total int
	db.QueryRow(`SELECT COUNT(*) FROM skill WHERE COALESCE(is_deleted,false)=false OR is_deleted IS NULL`).Scan(&total)
	fmt.Printf("total skills: %d\n\n", total)

	rows, err := db.Query(`SELECT skill_key, name, status, enabled, sync_origin FROM skill ORDER BY skill_key`)
	if err != nil {
		panic(err)
	}
	defer rows.Close()
	for rows.Next() {
		var k, n, st, so string
		var en bool
		rows.Scan(&k, &n, &st, &en, &so)
		fmt.Printf("%-32s %-40s status=%-10s enabled=%v origin=%s\n", k, trunc(n, 40), st, en, so)
	}
}

func trunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
