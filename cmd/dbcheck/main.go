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

	var count int
	db.QueryRow("SELECT COUNT(*) FROM teams").Scan(&count)
	fmt.Printf("teams count: %d\n", count)

	var aCount int
	db.QueryRow("SELECT COUNT(*) FROM agents").Scan(&aCount)
	fmt.Printf("agents count: %d\n", aCount)

	// Show teams
	rows, _ := db.Query("SELECT id, team_key FROM teams LIMIT 5")
	fmt.Println("\nSample teams:")
	for rows.Next() {
		var id, key string
		rows.Scan(&id, &key)
		fmt.Printf("  id=%s key=%s\n", id, key)
	}

	// Check a team's definition_json for agent IDs
	var defJSON string
	db.QueryRow("SELECT definition_json FROM teams WHERE team_key = 'team-fullstack-feature' LIMIT 1").Scan(&defJSON)
	if len(defJSON) > 300 {
		defJSON = defJSON[:300]
	}
	fmt.Printf("\nteam-fullstack-feature definition (first 300 chars):\n%s\n", defJSON)

	// Check schema_migrations
	rows2, _ := db.Query("SELECT version, name FROM schema_migrations WHERE name = 'industry_agents_v1'")
	fmt.Println("\nindustry_agents migrations:")
	for rows2.Next() {
		var v int
		var n string
		rows2.Scan(&v, &n)
		fmt.Printf("  version=%d name=%s\n", v, n)
	}
}
