package main

import (
	"database/sql"
	"fmt"
	"os"

	_ "github.com/lib/pq"
)

func main() {
	db, err := sql.Open("postgres", "postgres://postgres:Hangshan%40123@127.0.0.1:5432/aranea?sslmode=disable")
	if err != nil {
		panic(err)
	}
	defer db.Close()

	query := os.Args[1]
	out := os.Stdout
	if len(os.Args) > 2 {
		f, ferr := os.Create(os.Args[2])
		if ferr != nil {
			panic(ferr)
		}
		defer f.Close()
		out = f
	}
	rows, err := db.Query(query)
	if err != nil {
		fmt.Println("ERR:", err)
		os.Exit(1)
	}
	defer rows.Close()

	cols, _ := rows.Columns()
	fmt.Fprintln(out, "COLS:", cols)
	vals := make([]interface{}, len(cols))
	ptrs := make([]interface{}, len(cols))
	for i := range vals {
		ptrs[i] = &vals[i]
	}
	count := 0
	for rows.Next() {
		if err := rows.Scan(ptrs...); err != nil {
			fmt.Fprintln(out, "SCAN ERR:", err)
			continue
		}
		count++
		fmt.Fprintf(out, "ROW %d: ", count)
		for i, v := range vals {
			var s string
			switch b := v.(type) {
			case []byte:
				s = string(b)
			default:
				s = fmt.Sprintf("%v", v)
			}
			if len(s) > 4000 {
				s = s[:4000] + "...(truncated,len=" + fmt.Sprint(len(s)) + ")"
			}
			fmt.Fprintf(out, "[%s=%s] ", cols[i], s)
		}
		fmt.Fprintln(out)
	}
	fmt.Fprintln(out, "TOTAL:", count)
}
