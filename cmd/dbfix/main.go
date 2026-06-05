package main

import (
	"database/sql"
	"fmt"
	"os"

	_ "github.com/mattn/go-sqlite3"
)

func main() {
	dbPath := "./data/arenea.sqlite"
	if len(os.Args) > 1 {
		dbPath = os.Args[1]
	}
	db, err := sql.Open("sqlite3", dbPath+"?cache=shared&_fk=1")
	if err != nil {
		fmt.Fprintf(os.Stderr, "open db: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	// Show current state
	rows, err := db.Query("SELECT version, name FROM schema_migrations ORDER BY version")
	if err != nil {
		fmt.Fprintf(os.Stderr, "query migrations: %v\n", err)
	} else {
		fmt.Println("=== schema_migrations ===")
		for rows.Next() {
			var version int
			var name string
			rows.Scan(&version, &name)
			fmt.Printf("  version=%d name=%s\n", version, name)
		}
		rows.Close()
	}

	// Count taxonomy records
	var total, active, softDeleted int
	db.QueryRow("SELECT COUNT(*) FROM industry_taxonomy").Scan(&total)
	db.QueryRow("SELECT COUNT(*) FROM industry_taxonomy WHERE deleted_at = '' OR deleted_at IS NULL").Scan(&active)
	db.QueryRow("SELECT COUNT(*) FROM industry_taxonomy WHERE deleted_at IS NOT NULL AND deleted_at != ''").Scan(&softDeleted)
	fmt.Printf("=== industry_taxonomy: total=%d active=%d soft_deleted=%d ===\n", total, active, softDeleted)

	// Show all taxonomy records
	tRows, err := db.Query("SELECT id, taxonomy_key, name, level, deleted_at FROM industry_taxonomy ORDER BY id")
	if err != nil {
		fmt.Fprintf(os.Stderr, "query taxonomy: %v\n", err)
	} else {
		fmt.Println("=== industry_taxonomy records ===")
		for tRows.Next() {
			var id, key, name, level, deletedAt string
			tRows.Scan(&id, &key, &name, &level, &deletedAt)
			fmt.Printf("  id=%s key=%s name=%s level=%s deleted_at=%q\n", id, key, name, level, deletedAt)
		}
		tRows.Close()
	}

	// Delete the pack_builtin_v1 migration record so seed will re-run
	res, err := db.Exec("DELETE FROM schema_migrations WHERE version = 20260901")
	if err != nil {
		fmt.Fprintf(os.Stderr, "delete migration: %v\n", err)
		os.Exit(1)
	}
	affected, _ := res.RowsAffected()
	fmt.Printf("Deleted %d migration record(s) for version 20260901\n", affected)

	// Hard-delete any soft-deleted industry_taxonomy records
	res2, err := db.Exec("DELETE FROM industry_taxonomy WHERE deleted_at IS NOT NULL AND deleted_at != ''")
	if err != nil {
		fmt.Fprintf(os.Stderr, "delete soft-deleted taxonomy: %v\n", err)
		os.Exit(1)
	}
	affected2, _ := res2.RowsAffected()
	fmt.Printf("Hard-deleted %d soft-deleted industry_taxonomy record(s)\n", affected2)

	// Delete taxonomy records with empty ID (they cause UNIQUE constraint conflicts)
	res3, err := db.Exec("DELETE FROM industry_taxonomy WHERE id = '' OR id IS NULL")
	if err != nil {
		fmt.Fprintf(os.Stderr, "delete empty-id taxonomy: %v\n", err)
		os.Exit(1)
	}
	affected3, _ := res3.RowsAffected()
	fmt.Printf("Deleted %d taxonomy record(s) with empty ID\n", affected3)

	// Show admin users
	aRows, err := db.Query("SELECT id, name, email, access FROM admins")
	if err != nil {
		fmt.Fprintf(os.Stderr, "query admins: %v\n", err)
	} else {
		fmt.Println("=== admins ===")
		for aRows.Next() {
			var id int64
			var name, email, access string
			aRows.Scan(&id, &name, &email, &access)
			fmt.Printf("  id=%d name=%s email=%s access=%s\n", id, name, email, access)
		}
		aRows.Close()
	}

	fmt.Println("Done. Ready to restart server.")
}
