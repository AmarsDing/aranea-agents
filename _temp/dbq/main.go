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
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer db.Close()

	query := os.Args[1]
	rows, err := db.Query(query)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer rows.Close()

	cols, _ := rows.Columns()
	fmt.Println(cols)
	vals := make([]interface{}, len(cols))
	ptrs := make([]interface{}, len(cols))
	for i := range vals {
		ptrs[i] = &vals[i]
	}
	for rows.Next() {
		if err := rows.Scan(ptrs...); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		for i, v := range vals {
			s := ""
			switch t := v.(type) {
			case []byte:
				s = string(t)
			case string:
				s = t
			case nil:
				s = "NULL"
			default:
				s = fmt.Sprintf("%v", t)
			}
			if len(s) > 80 {
				s = s[:80] + "..."
			}
			fmt.Printf("%s=%q ", cols[i], s)
		}
		fmt.Println()
	}
}
