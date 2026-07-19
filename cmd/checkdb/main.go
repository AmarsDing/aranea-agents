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
		fmt.Println("open err:", err)
		os.Exit(1)
	}
	defer db.Close()

	for _, t := range []string{"learning_observations", "learning_patterns", "learning_proposals"} {
		var c int
		if err := db.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM %s", t)).Scan(&c); err != nil {
			fmt.Printf("TABLE %s: MISSING (%v)\n", t, err)
		} else {
			fmt.Printf("TABLE %s: OK rows=%d\n", t, c)
		}
	}
	var mig string
	err = db.QueryRow("SELECT name FROM schema_migrations WHERE version = 20261106").Scan(&mig)
	if err != nil {
		fmt.Println("migration 20261106: NOT RECORDED")
	} else {
		fmt.Println("migration 20261106:", mig)
	}
}
