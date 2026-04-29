package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite" // SQL driver for OpenSQLite
)

// OpenSQLite opens a single-connection SQLite pool for the given file path.
// The parent directory is created if missing. MaxOpenConns is set to 1, matching
// legacy repository and embedded-sqlite expectations.
func OpenSQLite(path string) (*sql.DB, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create db directory: %w", err)
	}
	database, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	database.SetMaxOpenConns(1)
	return database, nil
}
