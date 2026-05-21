// sqlmigrate applies a single docs/sql/*.sql file to the project SQLite DB (configs/data.source).
// Usage: go run ./cmd/sqlmigrate docs/sql/02_agent_code_executor_type.sql
package main

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "github.com/glebarez/go-sqlite"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintf(os.Stderr, "usage: go run ./cmd/sqlmigrate <path-to.sql>\n")
		os.Exit(2)
	}
	sqlPath := os.Args[1]
	body, err := os.ReadFile(sqlPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read sql: %v\n", err)
		os.Exit(1)
	}
	dbPath := defaultDBPath()
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open db %s: %v\n", dbPath, err)
		os.Exit(1)
	}
	defer db.Close()

	for _, stmt := range splitSQL(string(body)) {
		if _, err := db.Exec(stmt); err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
				fmt.Printf("skip (already applied): %s\n", stmt)
				continue
			}
			fmt.Fprintf(os.Stderr, "exec failed: %v\nstmt: %s\n", err, stmt)
			os.Exit(1)
		}
		fmt.Printf("ok: %s\n", stmt)
	}
	fmt.Printf("migration complete on %s\n", dbPath)
}

func defaultDBPath() string {
	if v := strings.TrimSpace(os.Getenv("ARANEA_SQLITE_PATH")); v != "" {
		return v
	}
	// matches configs/config.yaml data.sqlite.source file:./cmd/data/arenea.sqlite
	root, _ := os.Getwd()
	return filepath.Join(root, "cmd", "data", "arenea.sqlite")
}

func splitSQL(raw string) []string {
	var out []string
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "--") {
			continue
		}
		out = append(out, line)
	}
	if len(out) == 0 {
		return nil
	}
	// single-statement files: join lines
	return []string{strings.Join(out, " ")}
}
