// 查询 brand-guidelines 候选在 job 1b0eebbcc1fa7c54f4d38510 中的验证状态
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

	// 先看表结构
	cols, err := db.Query(`SELECT column_name FROM information_schema.columns WHERE table_name='skill_import_jobs' ORDER BY ordinal_position`)
	if err != nil {
		panic(err)
	}
	fmt.Println("== columns ==")
	for cols.Next() {
		var c string
		cols.Scan(&c)
		fmt.Println(" ", c)
	}
	cols.Close()

	// 取该 job 的 candidates_json 完整内容
	var cand, cg string
	err = db.QueryRow(`SELECT candidates_json::text, conflict_groups_json::text FROM skill_import_jobs WHERE id='1b0eebbcc1fa7c54f4d38510'`).Scan(&cand, &cg)
	if err != nil {
		panic(err)
	}
	fmt.Println("== candidates ==")
	fmt.Println(trunc(cand, 3000))
	fmt.Println("\n== conflict groups ==")
	fmt.Println(trunc(cg, 1500))

	// 同时列出所有含 brand-guidelines 的 job（含最新重试）
	fmt.Println("\n== all jobs with brand-guidelines ==")
	rows, err := db.Query(`SELECT id, status, validation_status, message, created_at FROM skill_import_jobs WHERE candidates_json::text LIKE '%brand-guidelines%' ORDER BY created_at DESC`)
	if err != nil {
		panic(err)
	}
	defer rows.Close()
	for rows.Next() {
		var id, st, vs, msg, ca string
		rows.Scan(&id, &st, &vs, &msg, &ca)
		fmt.Printf("job %s status=%s validation=%s created=%s msg=%s\n", id, st, vs, ca, msg)
	}
}

func trunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
