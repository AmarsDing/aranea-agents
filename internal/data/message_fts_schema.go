package data

import (
	"context"
	"database/sql"
	_ "embed"
	"strings"
)

//go:embed sql/message_fts.sql
var messageFTSDDL string

//go:embed sql/message_fts_postgres.sql
var messageFTSPostgresDDL string

// EnsureMessageFTSSchema creates messages_fts virtual table (SQLite) or tsvector
// column + GIN index (Postgres) and backfills rows.
func EnsureMessageFTSSchema(ctx context.Context, db *sql.DB) error {
	if db == nil {
		return nil
	}
	return EnsureMessageFTSSchemaWithDialect(ctx, db, DialectSQLite)
}

// EnsureMessageFTSSchemaWithDialect is the dialect-aware variant.
// SQLite: creates messages_fts FTS5 virtual table + triggers.
// Postgres: adds tsv GENERATED STORED tsvector column + GIN index.
func EnsureMessageFTSSchemaWithDialect(ctx context.Context, db *sql.DB, d Dialect) error {
	if db == nil {
		return nil
	}
	ddl := messageFTSDDL
	if d.IsPostgres() {
		ddl = messageFTSPostgresDDL
	}
	for _, stmt := range splitSQLStatements(ddl) {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			if d.AlreadyExistsErr(err) {
				continue
			}
			// Postgres DO $$ blocks may report "column already exists" via
			// different error codes; tolerate undefined_object for idempotency
			// when the table is created by a later migration.
			if d.IsPostgres() && isPostgresAlreadyExistsErr(err) {
				continue
			}
			// SQLite fallback: tolerate "already exists" in error message.
			if d.IsSQLite() && strings.Contains(err.Error(), "already exists") {
				continue
			}
			return err
		}
	}
	return nil
}

// splitSQLStatements splits DDL on semicolons outside BEGIN...END blocks (e.g. CREATE TRIGGER).
// Handles SQL line comments (-- to end of line) and Postgres $$ ... $$ dollar quoting.
func splitSQLStatements(ddl string) []string {
	ddl = strings.TrimSpace(ddl)
	if ddl == "" {
		return nil
	}
	var (
		out   []string
		cur   strings.Builder
		depth int // inside BEGIN...END
		// dollarQuote tracks whether we're inside a Postgres $$ ... $$ block.
		// Inside $$ blocks, semicolons are part of the function body and must
		// not be treated as statement separators.
		dollarQuote bool
	)
	flush := func() {
		s := strings.TrimSpace(cur.String())
		cur.Reset()
		if s != "" {
			out = append(out, s)
		}
	}
	for i := 0; i < len(ddl); {
		// Skip SQL line comments (-- to end of line). This prevents $$ or ;
		// inside comments from being interpreted as syntax.
		if !dollarQuote && i+1 < len(ddl) && ddl[i] == '-' && ddl[i+1] == '-' {
			// Find end of line.
			end := strings.IndexByte(ddl[i:], '\n')
			if end < 0 {
				// Comment runs to end of input.
				cur.WriteString(ddl[i:])
				break
			}
			cur.WriteString(ddl[i : i+end+1])
			i += end + 1
			continue
		}
		// Detect Postgres $$ ... $$ quoting (used in DO $$ blocks).
		if !dollarQuote && i+1 < len(ddl) && ddl[i] == '$' && ddl[i+1] == '$' {
			dollarQuote = true
			cur.WriteString("$$")
			i += 2
			continue
		}
		if dollarQuote && i+1 < len(ddl) && ddl[i] == '$' && ddl[i+1] == '$' {
			dollarQuote = false
			cur.WriteString("$$")
			i += 2
			continue
		}
		if dollarQuote {
			cur.WriteByte(ddl[i])
			i++
			continue
		}
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
