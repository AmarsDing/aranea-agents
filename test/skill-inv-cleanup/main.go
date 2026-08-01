// 一次性清理脚本：删除 skill_invocation 中 filesystem_* 同步噪音记录。
// 背景：文件系统同步（scan/watch/reconcile）会为每个 skill 写入 skill_invocation
// 记录，但 agent_id 为空、不计入真实调用。统计 SQL 已加 source='runtime' 过滤，
// 此处物理清理历史噪音（用户已批准）。幂等，重复执行安全。
package main

import (
	"database/sql"
	"fmt"
	"os"

	_ "github.com/lib/pq"
)

func main() {
	dsn := "postgres://postgres:Hangshan%40123@127.0.0.1:5432/aranea?sslmode=disable"
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		fmt.Println("open:", err)
		os.Exit(1)
	}
	defer db.Close()

	tx, err := db.Begin()
	if err != nil {
		fmt.Println("begin:", err)
		os.Exit(1)
	}
	defer tx.Rollback()

	var before int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM skill_invocation`).Scan(&before); err != nil {
		fmt.Println("count before:", err)
		os.Exit(1)
	}

	res, err := tx.Exec(`DELETE FROM skill_invocation WHERE source IN ('filesystem_scan','filesystem_watch','filesystem_reconcile')`)
	if err != nil {
		fmt.Println("delete:", err)
		os.Exit(1)
	}
	deleted, _ := res.RowsAffected()

	var after, runtimeLeft int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM skill_invocation`).Scan(&after); err != nil {
		fmt.Println("count after:", err)
		os.Exit(1)
	}
	if err := tx.QueryRow(`SELECT COUNT(*) FROM skill_invocation WHERE source='runtime'`).Scan(&runtimeLeft); err != nil {
		fmt.Println("count runtime:", err)
		os.Exit(1)
	}

	if err := tx.Commit(); err != nil {
		fmt.Println("commit:", err)
		os.Exit(1)
	}
	fmt.Printf("before=%d deleted=%d after=%d runtime_left=%d\n", before, deleted, after, runtimeLeft)
}
