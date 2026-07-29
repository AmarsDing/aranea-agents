// 查 3 个冲突 slug 的库内记录
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

	slugs := []string{
		"alibabacloud-alinux-sysom-inspection",
		"alibabacloud-sls-query",
		"alibabacloud-network-health-inspection",
		"alibabacloud-oos-chatops-agent",
	}
	for _, s := range slugs {
		rows, err := db.Query(`SELECT id, status, lifecycle_status, enabled, created_at FROM skill WHERE skill_key=$1`, s)
		if err != nil {
			fmt.Printf("%s: query err %v\n", s, err)
			continue
		}
		found := false
		for rows.Next() {
			var id, st, lc, ca string
			var en bool
			rows.Scan(&id, &st, &lc, &en, &ca)
			fmt.Printf("%s → id=%s status=%s lifecycle=%s enabled=%v created=%s\n", s, id, st, lc, en, ca)
			found = true
		}
		rows.Close()
		if !found {
			fmt.Printf("%s → 无记录\n", s)
		}
	}

	// 全量 alibabacloud-* 当前状态
	fmt.Println("\n== 全部 alibabacloud-* ==")
	rows2, _ := db.Query(`SELECT skill_key, status, enabled, lifecycle_status, filesystem_missing FROM skill WHERE skill_key LIKE 'alibabacloud%' ORDER BY skill_key`)
	defer rows2.Close()
	for rows2.Next() {
		var k, st, lc string
		var en, fm bool
		rows2.Scan(&k, &st, &en, &lc, &fm)
		fmt.Printf("%-44s %-10s enabled=%-5v lifecycle=%-10s fs_missing=%v\n", k, st, en, lc, fm)
	}
}
