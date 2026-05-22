// pgprobe prints Postgres server version for a DSN (no app init side effects).
// Usage: go run ./cmd/pgprobe "postgres://..."
package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"

	_ "github.com/lib/pq"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: go run ./cmd/pgprobe <dsn>")
		os.Exit(2)
	}
	db, err := sql.Open("postgres", os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer db.Close()
	var ver string
	if err := db.QueryRowContext(context.Background(), `SELECT version()`).Scan(&ver); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	var ext string
	_ = db.QueryRowContext(context.Background(),
		`SELECT string_agg(extname, ', ') FROM pg_extension`).Scan(&ext)
	fmt.Println(ver)
	fmt.Println("extensions:", ext)
}
