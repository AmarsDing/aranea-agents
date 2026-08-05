package main

import (
	"database/sql"
	"fmt"
	"os"

	_ "github.com/lib/pq"
)

// One-off inspection: where does monitor_traces domain actually live?
func main() {
	dsn := "postgres://postgres:Hangshan%40123@127.0.0.1:5432/aranea?sslmode=disable" // dev DB
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		fmt.Println("open:", err)
		os.Exit(1)
	}
	defer db.Close()

	rows, err := db.Query(`
SELECT COALESCE(NULLIF(COALESCE(NULLIF(metadata_json::text, '')::jsonb, '{}'::jsonb) ->> 'domain', ''),'(empty)') AS meta_domain,
       COUNT(*),
       COUNT(*) FILTER (WHERE metadata_json = '' OR metadata_json IS NULL) AS empty_meta
FROM monitor_traces
WHERE deleted_at IS NULL OR deleted_at = ''
GROUP BY 1 ORDER BY 2 DESC`)
	if err != nil {
		fmt.Println("query:", err)
		os.Exit(1)
	}
	defer rows.Close()
	fmt.Println("metadata_json.domain | count | empty_metadata_rows")
	for rows.Next() {
		var d string
		var c, e int
		if err := rows.Scan(&d, &c, &e); err != nil {
			fmt.Println("scan:", err)
			os.Exit(1)
		}
		fmt.Printf("%-20s %6d %6d\n", d, c, e)
	}

	// distinct stored names (ensureTrace writes Name=domain at insert)
	r2, err := db.Query(`
SELECT name, COUNT(*) FROM monitor_traces
WHERE deleted_at IS NULL OR deleted_at = ''
GROUP BY name ORDER BY 2 DESC LIMIT 15`)
	if err != nil {
		fmt.Println("query2:", err)
		os.Exit(1)
	}
	defer r2.Close()
	fmt.Println("\nstored name distribution:")
	for r2.Next() {
		var name string
		var c int
		if err := r2.Scan(&name, &c); err != nil {
			fmt.Println("scan2:", err)
			os.Exit(1)
		}
		fmt.Printf("%-30s %6d\n", name, c)
	}
}
