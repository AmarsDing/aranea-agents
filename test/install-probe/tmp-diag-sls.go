// 深挖 sls-query 最新 job 冲突详情
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

	// 当前 sls-query 库内记录
	fmt.Println("== sls-query 库内记录 ==")
	rows, _ := db.Query(`SELECT id, status, lifecycle_status, created_at FROM skill WHERE skill_key LIKE '%sls%'`)
	for rows.Next() {
		var id, st, lc, ca string
		rows.Scan(&id, &st, &lc, &ca)
		fmt.Printf("id=%s status=%s lifecycle=%s created=%s\n", id, st, lc, ca)
	}
	rows.Close()

	// 最新 job 的 candidates + groups 完整内容
	fmt.Println("\n== 最新 job 8cf1face5142cd8c44181307 ==")
	var cands, groups string
	err = db.QueryRow(`SELECT candidates_json::text, conflict_groups_json::text FROM skill_import_jobs WHERE id='8cf1face5142cd8c44181307'`).Scan(&cands, &groups)
	if err != nil {
		fmt.Println("err:", err)
		return
	}
	fmt.Printf("candidates: %s\n\n", cands)
	fmt.Printf("groups: %s\n", groups)
}
