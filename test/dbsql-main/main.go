// dbsql-main is a throwaway debugging tool that runs a single SQL statement
// against the main aranea Postgres database and prints rows in a simple
// pipe-separated format. DSN is read from PG_DSN env var, defaulting to the
// local dev database from configs/config.yaml.
//
// Usage:
//
//	go run ./test/dbsql-main "SELECT 1;"
//	bin\dbsql-main.exe "SELECT 1;"
package main

import (
	"database/sql"
	"fmt"
	"os"
	"strings"

	_ "github.com/lib/pq"
)

func main() {
	dsn := os.Getenv("PG_DSN")
	if dsn == "" {
		dsn = "postgres://postgres:Hangshan%40123@127.0.0.1:5432/aranea?sslmode=disable"
	}
	if len(os.Args) < 2 || strings.TrimSpace(os.Args[1]) == "" {
		fmt.Fprintln(os.Stderr, "usage: dbsql-main <sql>")
		os.Exit(2)
	}
	query := os.Args[1]

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open error: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	rows, err := db.Query(query)
	if err != nil {
		fmt.Fprintf(os.Stderr, "query error: %v\n", err)
		os.Exit(1)
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		fmt.Fprintf(os.Stderr, "columns error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("COLS: %s\n", strings.Join(cols, " | "))

	n := 0
	vals := make([]sql.NullString, len(cols))
	ptrs := make([]any, len(cols))
	for i := range vals {
		ptrs[i] = &vals[i]
	}
	for rows.Next() {
		if err := rows.Scan(ptrs...); err != nil {
			fmt.Fprintf(os.Stderr, "scan error: %v\n", err)
			os.Exit(1)
		}
		parts := make([]string, len(cols))
		for i, v := range vals {
			if v.Valid {
				s := v.String
				if len(s) > 200 {
					s = s[:200] + "…"
				}
				parts[i] = s
			} else {
				parts[i] = "NULL"
			}
		}
		fmt.Println(strings.Join(parts, " | "))
		n++
	}
	if err := rows.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "rows error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("ROWS: %d\n", n)
}
