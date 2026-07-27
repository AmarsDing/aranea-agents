package main

import (
	"database/sql"
	"fmt"

	_ "github.com/lib/pq"
)

func main() {
	db, err := sql.Open("postgres", "postgres://postgres:Hangshan%40123@127.0.0.1:5432/aranea?sslmode=disable")
	if err != nil {
		panic(err)
	}
	defer db.Close()

	rows, err := db.Query(`SELECT table_name FROM information_schema.tables WHERE table_schema='public' AND (table_name LIKE 'learning%' OR table_name LIKE '%quota%' OR table_name LIKE '%budget%') ORDER BY table_name`)
	if err != nil {
		panic(err)
	}
	fmt.Println("== tables (learning/quota/budget) ==")
	for rows.Next() {
		var n string
		rows.Scan(&n)
		fmt.Println(" ", n)
	}
	rows.Close()

	rows2, err := db.Query(`SELECT version, name, applied_at FROM schema_migrations ORDER BY version DESC LIMIT 12`)
	if err != nil {
		fmt.Println("schema_migrations query error:", err)
		return
	}
	fmt.Println("== schema_migrations (latest 12) ==")
	for rows2.Next() {
		var v int64
		var n, ts string
		rows2.Scan(&v, &n, &ts)
		fmt.Printf("  %d  %-40s %s\n", v, n, ts)
	}
	rows2.Close()
}
