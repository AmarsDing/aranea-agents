package main

import (
	"database/sql"
	"fmt"

	_ "github.com/lib/pq"
)

func main() {
	dsn := "postgres://postgres:Hangshan%40123@127.0.0.1:5432/aranea?sslmode=disable"
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		fmt.Println("open:", err)
		return
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		fmt.Println("ping:", err)
		return
	}

	orderBy := `is_default DESC,
		CASE WHEN kind = 'system_builtin' AND agent_variant <> 'dept_lead' THEN 0 ELSE 1 END,
		kind DESC, updated_at DESC, id ASC`

	fmt.Println("== first 10 rows with NEW ordering ==")
	rows, err := db.Query(`SELECT display_name, kind, agent_variant FROM agents WHERE deleted_at = ''
		ORDER BY ` + orderBy + ` LIMIT 10`)
	if err != nil {
		fmt.Println(err)
		return
	}
	for rows.Next() {
		var name, kind, variant string
		rows.Scan(&name, &kind, &variant)
		fmt.Printf("  %-22s kind=%-18s variant=%q\n", name, kind, variant)
	}
	rows.Close()

	// Full pagination coverage with the NEW ordering.
	const pageSize = 21
	var total int
	db.QueryRow(`SELECT count(*) FROM agents WHERE deleted_at = ''`).Scan(&total)
	seen := map[string]bool{}
	fetched, dups := 0, 0
	for offset := 0; ; offset += pageSize {
		rows, err := db.Query(`SELECT id FROM agents WHERE deleted_at = ''
			ORDER BY ` + orderBy + ` LIMIT $1 OFFSET $2`, pageSize, offset)
		if err != nil {
			fmt.Println("page query:", err)
			return
		}
		n := 0
		for rows.Next() {
			var id string
			rows.Scan(&id)
			if seen[id] {
				dups++
			}
			seen[id] = true
			n++
			fetched++
		}
		rows.Close()
		if n < pageSize {
			break
		}
	}
	fmt.Printf("total=%d fetched=%d unique=%d duplicates=%d missing=%d\n", total, fetched, len(seen), dups, total-len(seen))
	if len(seen) == total && dups == 0 {
		fmt.Println("PAGINATION OK")
	} else {
		fmt.Println("PAGINATION BROKEN")
	}

	fmt.Println("\n== migration 20261112 ==")
	var migCount int
	db.QueryRow(`SELECT count(*) FROM schema_migrations WHERE version = 20261112`).Scan(&migCount)
	fmt.Printf("schema_migrations version=20261112: %d row(s)\n", migCount)

	var orgDisabled, orgTotal int
	db.QueryRow(`SELECT count(*) FROM organizations`).Scan(&orgTotal)
	db.QueryRow(`SELECT count(*) FROM organizations WHERE enabled = FALSE`).Scan(&orgDisabled)
	fmt.Printf("organizations: total=%d disabled=%d\n", orgTotal, orgDisabled)

	var copyEmpty int
	db.QueryRow(`SELECT count(*) FROM agents WHERE agent_key LIKE '%-copy-%' AND position_key = '' AND deleted_at = ''`).Scan(&copyEmpty)
	fmt.Printf("copy agents with empty position_key: %d\n", copyEmpty)

	if migCount == 1 && orgDisabled == 0 && copyEmpty == 0 {
		fmt.Println("MIGRATION OK")
	} else {
		fmt.Println("MIGRATION INCOMPLETE")
	}

	fmt.Println("\n== organizations by level ==")
	lvlRows, err := db.Query(`SELECT level, count(*), sum(CASE WHEN enabled THEN 1 ELSE 0 END) FROM organizations GROUP BY level ORDER BY level`)
	if err != nil {
		fmt.Println(err)
		return
	}
	for lvlRows.Next() {
		var lvl string
		var cnt, en int
		lvlRows.Scan(&lvl, &cnt, &en)
		fmt.Printf("  level=%-12s total=%d enabled=%d\n", lvl, cnt, en)
	}
	lvlRows.Close()

	fmt.Println("\n== company nodes (industry filter source) ==")
	coRows, err := db.Query(`SELECT name, key FROM organizations WHERE level = 'company' AND enabled = TRUE ORDER BY name LIMIT 10`)
	if err != nil {
		fmt.Println(err)
		return
	}
	for coRows.Next() {
		var name, key string
		coRows.Scan(&name, &key)
		fmt.Printf("  %-20s key=%s\n", name, key)
	}
	coRows.Close()
}
