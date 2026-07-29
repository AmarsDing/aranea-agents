// 清理 ecs-diagnose deleted 墓碑记录
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

	// 先删版本记录（外键）
	res1, err := db.Exec(`DELETE FROM skill_version WHERE skill_id IN (SELECT id FROM skill WHERE skill_key='alibabacloud-ecs-diagnose' AND status='deleted')`)
	if err != nil {
		fmt.Println("del versions err:", err)
	} else {
		n, _ := res1.RowsAffected()
		fmt.Println("deleted skill_version rows:", n)
	}
	res2, err := db.Exec(`DELETE FROM skill WHERE skill_key='alibabacloud-ecs-diagnose' AND status='deleted'`)
	if err != nil {
		fmt.Println("del skill err:", err)
		return
	}
	n, _ := res2.RowsAffected()
	fmt.Println("deleted skill rows:", n)
}
