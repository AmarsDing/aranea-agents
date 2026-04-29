package db

import (
	"database/sql"
	"strings"
)

// MigrateWithLegacyHook applies embedded schema in two Exec phases: first the
// table DDL (slice before the first CREATE INDEX), then afterTables (e.g.
// ADD COLUMN migrations for old installs), then the full script including indexes.
// This matches the historical SQLiteRepository.Migrate ordering.
func MigrateWithLegacyHook(db *sql.DB, schemaText string, afterTables func() error) error {
	tableSchema := schemaText
	if idx := strings.Index(schemaText, "CREATE INDEX"); idx >= 0 {
		tableSchema = schemaText[:idx]
	}
	if _, err := db.Exec(tableSchema); err != nil {
		return err
	}
	if afterTables != nil {
		if err := afterTables(); err != nil {
			return err
		}
	}
	_, err := db.Exec(schemaText)
	return err
}
