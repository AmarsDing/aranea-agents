package data

import (
	"context"
	"database/sql"
	_ "embed"
	"strings"
)

//go:embed sql/message_fts.sql
var messageFTSDDL string

// EnsureMessageFTSSchema creates messages_fts virtual table and backfills rows.
func EnsureMessageFTSSchema(ctx context.Context, db *sql.DB) error {
	if db == nil {
		return nil
	}
	for _, stmt := range splitSQLStatements(messageFTSDDL) {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			if strings.Contains(err.Error(), "already exists") {
				continue
			}
			return err
		}
	}
	return nil
}

func splitSQLStatements(ddl string) []string {
	var out []string
	for _, part := range strings.Split(ddl, ";") {
		s := strings.TrimSpace(part)
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}
