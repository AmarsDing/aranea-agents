// 查询成员会话的消息内容（先探查表名）
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

	rows, err := db.Query(`SELECT table_name FROM information_schema.tables WHERE table_schema='public' AND (table_name LIKE '%skill%' OR table_name LIKE '%import%') ORDER BY table_name`)
	if err != nil {
		panic(err)
	}
	defer rows.Close()
	for rows.Next() {
		var t string
		rows.Scan(&t)
		fmt.Println(t)
	}
}
