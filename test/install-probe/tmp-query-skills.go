// 查询 skills 表当前状态
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

	var total int
	if err := db.QueryRow(`SELECT COUNT(*) FROM skill`).Scan(&total); err != nil {
		panic(err)
	}
	fmt.Printf("total skills: %d\n\n", total)

	cols, _ := db.Query(`SELECT column_name FROM information_schema.columns WHERE table_name='skill' ORDER BY ordinal_position`)
	fmt.Print("columns: ")
	for cols.Next() {
		var c string
		cols.Scan(&c)
		fmt.Print(c, " ")
	}
	cols.Close()
	fmt.Println()

	rows, err := db.Query(`SELECT * FROM skill ORDER BY created_at DESC`)
	if err != nil {
		panic(err)
	}
	defer rows.Close()
	colNames, _ := rows.Columns()
	fmt.Println("\nrows:")
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
			b, ok := v.([]byte)
			if ok {
				v = string(b)
			}
			s := fmt.Sprintf("%v", v)
			if len(s) > 120 {
				s = s[:120] + "…"
			}
			fmt.Printf("  %s = %s\n", cn, s)
		}
	}
}
