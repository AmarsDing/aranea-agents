// 查询最近的 skill 导入 job 及其冲突详情
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

	cols, _ := db.Query(`SELECT column_name FROM information_schema.columns WHERE table_name='skill_import_jobs' ORDER BY ordinal_position`)
	fmt.Print("columns: ")
	for cols.Next() {
		var c string
		cols.Scan(&c)
		fmt.Print(c, " ")
	}
	cols.Close()
	fmt.Println("\n")

	rows, err := db.Query(`SELECT * FROM skill_import_jobs ORDER BY created_at DESC LIMIT 20`)
	if err != nil {
		panic(err)
	}
	defer rows.Close()
	colNames, _ := rows.Columns()
	for rows.Next() {
		vals := make([]any, len(colNames))
		ptrs := make([]any, len(colNames))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		rows.Scan(ptrs...)
		fmt.Println("---")
		for i, cn := range colNames {
			v := vals[i]
			if b, ok := v.([]byte); ok {
				v = string(b)
			}
			s := fmt.Sprintf("%v", v)
			if len(s) > 600 {
				s = s[:600] + "…"
			}
			if s == "" {
				continue
			}
			fmt.Printf("  %s = %s\n", cn, s)
		}
	}
}
