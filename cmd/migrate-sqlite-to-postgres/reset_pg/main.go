package main

import (
	"database/sql"
	"flag"
	"fmt"
	"os"
	"strings"

	_ "github.com/lib/pq"
)

func main() {
	// Postgres DSN for the maintenance connection (connects to 'postgres' db
	// to drop/create the target database). Required — never hardcode credentials (red line #25).
	// Set --dsn or ARANEA_PG_ADMIN_DSN env var.
	dsn := flag.String("dsn", "", "Postgres admin DSN (connects to 'postgres' db), or set ARANEA_PG_ADMIN_DSN env var")
	dbName := flag.String("db", "aranea", "Database name to drop and recreate")
	flag.Parse()

	connStr := *dsn
	if connStr == "" {
		connStr = os.Getenv("ARANEA_PG_ADMIN_DSN")
	}
	if connStr == "" {
		fmt.Fprintln(os.Stderr, "postgres admin DSN is required: pass --dsn or set ARANEA_PG_ADMIN_DSN env var")
		os.Exit(2)
	}

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	if _, err := db.Exec(fmt.Sprintf(`DROP DATABASE IF EXISTS %s`, quoteIdent(*dbName))); err != nil {
		fmt.Fprintf(os.Stderr, "drop: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("dropped %s\n", *dbName)

	if _, err := db.Exec(fmt.Sprintf(`CREATE DATABASE %s`, quoteIdent(*dbName))); err != nil {
		fmt.Fprintf(os.Stderr, "create: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("created %s\n", *dbName)
}

// quoteIdent wraps a Postgres identifier in double quotes, escaping any
// internal double quotes by doubling them (per SQL standard). This prevents
// SQL injection on the database name, which cannot be parameterized in
// DROP/CREATE DATABASE statements.
func quoteIdent(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}
