// 验证 alibabacloud-find-skills 安装状态
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

	var id, name, slug, status, lifecycle string
	var enabled, fsMissing bool
	err = db.QueryRow(`SELECT id, name, skill_key, status, enabled, filesystem_missing, lifecycle_status FROM skill WHERE skill_key LIKE '%alibabacloud%'`).Scan(&id, &name, &slug, &status, &enabled, &fsMissing, &lifecycle)
	if err != nil {
		fmt.Println("query err:", err)
		return
	}
	fmt.Printf("id=%s\nname=%s\nslug=%s\nstatus=%s enabled=%v filesystem_missing=%v lifecycle=%s\n", id, name, slug, status, enabled, fsMissing, lifecycle)

	// 当前 skill 总数与启用数
	var total, enabledCnt int
	db.QueryRow(`SELECT COUNT(*), COUNT(*) FILTER (WHERE enabled) FROM skill`).Scan(&total, &enabledCnt)
	fmt.Printf("\ntotal skills=%d enabled=%d\n", total, enabledCnt)
}
