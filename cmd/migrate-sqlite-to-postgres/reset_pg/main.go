package main

import (
	"database/sql"
	"fmt"
	"os"

	_ "github.com/lib/pq"
)

func main() {
	dsn := "postgres://postgres:Hangshan%40123@127.0.0.1:5432/postgres?sslmode=disable"
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	if _, err := db.Exec(`DROP DATABASE IF EXISTS aranea`); err != nil {
		fmt.Fprintf(os.Stderr, "drop: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("dropped aranea")

	if _, err := db.Exec(`CREATE DATABASE aranea`); err != nil {
		fmt.Fprintf(os.Stderr, "create: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("created aranea")
}
