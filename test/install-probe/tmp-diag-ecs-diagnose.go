// 排查 ecs-diagnose deleted 原因 + 查看导入 job 状态
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

	fmt.Println("== skill 表列 ==")
	colRows, _ := db.Query(`SELECT column_name FROM information_schema.columns WHERE table_name='skill' ORDER BY ordinal_position`)
	var cols []string
	for colRows.Next() {
		var c string
		colRows.Scan(&c)
		cols = append(cols, c)
	}
	colRows.Close()
	fmt.Println(cols)

	fmt.Println("\n== skill_import_jobs 表列 ==")
	colRows2, err := db.Query(`SELECT column_name FROM information_schema.columns WHERE table_name='skill_import_jobs' ORDER BY ordinal_position`)
	if err != nil {
		fmt.Println("err:", err)
	} else {
		var cols2 []string
		for colRows2.Next() {
			var c string
			colRows2.Scan(&c)
			cols2 = append(cols2, c)
		}
		colRows2.Close()
		fmt.Println(cols2)
	}

	fmt.Println("\n== ecs-diagnose 记录 ==")
	rows, err := db.Query(`SELECT id, skill_key, status, enabled, lifecycle_status, created_at, updated_at FROM skill WHERE skill_key LIKE '%ecs-diagnose%' ORDER BY created_at`)
	if err != nil {
		fmt.Println("err:", err)
	} else {
		defer rows.Close()
		for rows.Next() {
			var id, k, st, lc, ca, ua string
			var en bool
			rows.Scan(&id, &k, &st, &en, &lc, &ca, &ua)
			fmt.Printf("id=%s key=%s status=%s enabled=%v lifecycle=%s created=%s updated=%s\n", id, k, st, en, lc, ca, ua)
		}
	}

	fmt.Println("\n== skill 表索引 ==")
	idxRows, err := db.Query(`SELECT indexname, indexdef FROM pg_indexes WHERE tablename='skill'`)
	if err != nil {
		fmt.Println("idx err:", err)
	} else {
		defer idxRows.Close()
		for idxRows.Next() {
			var n, d string
			idxRows.Scan(&n, &d)
			fmt.Printf("%s: %s\n", n, d)
		}
	}
}

func trunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
