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

// splitSQLStatements splits DDL on semicolons outside BEGIN...END blocks (e.g. CREATE TRIGGER).
func splitSQLStatements(ddl string) []string {
	ddl = strings.TrimSpace(ddl)
	if ddl == "" {
		return nil
	}
	var (
		out   []string
		cur   strings.Builder
		depth int // inside BEGIN...END
	)
	flush := func() {
		s := strings.TrimSpace(cur.String())
		cur.Reset()
		if s != "" {
			out = append(out, s)
		}
	}
	for i := 0; i < len(ddl); {
		if depth == 0 && matchSQLWord(ddl, i, "BEGIN") {
			depth++
			cur.WriteString(ddl[i : i+5])
			i += 5
			continue
		}
		if depth > 0 && matchSQLWord(ddl, i, "END") {
			depth--
			cur.WriteString(ddl[i : i+3])
			i += 3
			continue
		}
		if ddl[i] == ';' && depth == 0 {
			flush()
			i++
			continue
		}
		cur.WriteByte(ddl[i])
		i++
	}
	flush()
	return out
}

func matchSQLWord(s string, i int, word string) bool {
	if i+len(word) > len(s) || !strings.EqualFold(s[i:i+len(word)], word) {
		return false
	}
	return sqlWordBoundaryBefore(s, i) && sqlWordBoundaryAfter(s, i+len(word))
}

func sqlWordBoundaryBefore(s string, i int) bool {
	return i == 0 || !isSQLIdentByte(s[i-1])
}

func sqlWordBoundaryAfter(s string, i int) bool {
	return i >= len(s) || !isSQLIdentByte(s[i])
}

func isSQLIdentByte(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') || b == '_'
}
