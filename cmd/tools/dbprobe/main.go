package main

import (
	"database/sql"
	"fmt"

	_ "github.com/lib/pq"
)

func main() {
	dsn := "postgres://postgres:Hangshan%40123@127.0.0.1:5432/aranea?sslmode=disable"
	db, _ := sql.Open("postgres", dsn)
	defer db.Close()

	fmt.Println("== org nodes created_at range ==")
	var minC, maxC string
	db.QueryRow(`SELECT min(created_at), max(created_at) FROM organizations`).Scan(&minC, &maxC)
	fmt.Println("  min:", minC, "max:", maxC)

	fmt.Println("== seed migrations applied ==")
	rows, err := db.Query(`SELECT version, name, applied_at FROM schema_migrations ORDER BY applied_at DESC LIMIT 30`)
	if err != nil { fmt.Println(err); return }
	for rows.Next() {
		var v, n, a string
		rows.Scan(&v, &n, &a)
		fmt.Printf("  %-16s %-40s %s\n", v, n, a)
	}
	rows.Close()
}
