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

	// Delete the version record
	res, err := db.Exec("DELETE FROM schema_migrations WHERE name = 'industry_agents_v1'")
	if err != nil {
		fmt.Printf("DELETE error: %v\n", err)
		return
	}
	affected, _ := res.RowsAffected()
	fmt.Printf("Deleted %d rows from schema_migrations\n", affected)

	// Also clear teams table to start fresh
	res2, err := db.Exec("DELETE FROM teams")
	if err != nil {
		fmt.Printf("DELETE teams error: %v\n", err)
		return
	}
	affected2, _ := res2.RowsAffected()
	fmt.Printf("Deleted %d rows from teams\n", affected2)

	// Show remaining migrations
	rows, err := db.Query("SELECT name, version FROM schema_migrations")
	if err != nil {
		fmt.Printf("SELECT error: %v\n", err)
		return
	}
	defer rows.Close()
	fmt.Println("Remaining migrations:")
	for rows.Next() {
		var name string
		var ver int
		rows.Scan(&name, &ver)
		fmt.Printf("  %s: %d\n", name, ver)
	}
}
