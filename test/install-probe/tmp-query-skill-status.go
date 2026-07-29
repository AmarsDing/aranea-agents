// 查看全部 skill 的 status/enabled 分布
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

	rows, err := db.Query(`SELECT skill_key, status, enabled, lifecycle_status, filesystem_missing FROM skill ORDER BY created_at`)
	if err != nil {
		panic(err)
	}
	defer rows.Close()
	fmt.Printf("%-40s %-12s %-8s %-10s %s\n", "slug", "status", "enabled", "lifecycle", "fs_missing")
	for rows.Next() {
		var k, st, lc string
		var en, fm bool
		rows.Scan(&k, &st, &en, &lc, &fm)
		fmt.Printf("%-40s %-12s %-8v %-10s %v\n", k, st, en, lc, fm)
	}
}
