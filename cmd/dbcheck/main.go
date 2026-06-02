package main

import (
	"database/sql"
	"fmt"
	_ "github.com/mattn/go-sqlite3"
)

func main() {
	db, err := sql.Open("sqlite3", "data/arenea.sqlite")
	if err != nil {
		panic(err)
	}
	defer db.Close()

	// Delete old industry_agents version records so seed can re-run
	result, err := db.Exec("DELETE FROM schema_migrations WHERE version >= 20260802 AND name = 'industry_agents_v1'")
	if err != nil {
		fmt.Printf("Error deleting migrations: %v\n", err)
	} else {
		affected, _ := result.RowsAffected()
		fmt.Printf("Deleted %d industry_agents migration records\n", affected)
	}

	var count int
	db.QueryRow("SELECT COUNT(*) FROM teams").Scan(&count)
	fmt.Printf("teams count: %d\n", count)

	var aCount int
	db.QueryRow("SELECT COUNT(*) FROM agents").Scan(&aCount)
	fmt.Printf("agents count: %d\n", aCount)

	// Check remaining schema_migrations
	rows, _ := db.Query("SELECT version, name FROM schema_migrations WHERE version >= 20260800 ORDER BY version")
	fmt.Println("\nRemaining migrations >= 20260800:")
	for rows.Next() {
		var v int
		var n string
		rows.Scan(&v, &n)
		fmt.Printf("  migration: %d %s\n", v, n)
	}
}
