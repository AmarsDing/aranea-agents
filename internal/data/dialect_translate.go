package data

import "strings"

// translateSQLiteDDLToPostgres translates SQLite-specific DDL syntax in the
// given SQL string to Postgres-compatible equivalents.
//
// Translations applied:
//   - "INTEGER PRIMARY KEY AUTOINCREMENT" -> "SERIAL PRIMARY KEY"
//     (SQLite AUTOINCREMENT creates an auto-incrementing rowid; Postgres SERIAL
//     creates an INTEGER column backed by a sequence.)
//   - "BLOB" -> "BYTEA"
//     (SQLite BLOB is a binary blob; Postgres BYTEA is the equivalent.)
//   - "strftime('%Y-%m-%dT%H:%M:%SZ', 'now')" -> Postgres to_char equivalent
//     (SQLite strftime for ISO 8601 UTC timestamp; Postgres uses to_char.)
//
// Note: "INSERT OR IGNORE INTO" is handled per-statement in
// executeSQLFileWithDialect because it requires appending "ON CONFLICT DO
// NOTHING" at the end of each INSERT statement.
//
// The translation is intentionally conservative: it only rewrites tokens that
// are unambiguously SQLite-specific. Complex constructs (PRAGMA, sqlite_master,
// json_each, etc.) are not handled here — those should use the Dialect methods
// (JSONEach, TableExistsQuery, etc.) at the call site instead.
//
// This function is called by executeSQLFileWithDialect when the active dialect
// is Postgres, to allow shared SQL migration files to work on both databases.
func translateSQLiteDDLToPostgres(ddl string) string {
	// INTEGER PRIMARY KEY AUTOINCREMENT -> SERIAL PRIMARY KEY
	// Match case-insensitively but preserve the rest of the line.
	ddl = replaceCaseInsensitive(ddl, "INTEGER PRIMARY KEY AUTOINCREMENT", "SERIAL PRIMARY KEY")
	// BLOB -> BYTEA (only as a column type, not inside strings).
	// This is safe because BLOB is not a valid identifier and only appears as
	// a column type in DDL.
	ddl = replaceCaseInsensitive(ddl, "BLOB", "BYTEA")
	// strftime('%Y-%m-%dT%H:%M:%SZ', 'now') -> to_char(now() AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
	// SQLite strftime for ISO 8601 UTC timestamp; Postgres uses to_char with
	// a different format string.
	ddl = replaceCaseInsensitive(ddl,
		`strftime('%Y-%m-%dT%H:%M:%SZ', 'now')`,
		`to_char(now() AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"')`)
	return ddl
}

// translateSQLiteStatementToPostgres translates a single SQLite SQL statement
// to Postgres-compatible form. This is called per-statement after splitting,
// to handle constructs that require statement-level rewriting (e.g. INSERT OR
// IGNORE -> INSERT ... ON CONFLICT DO NOTHING).
func translateSQLiteStatementToPostgres(stmt string) string {
	// INSERT OR IGNORE INTO -> INSERT INTO ... ON CONFLICT DO NOTHING
	// SQLite: INSERT OR IGNORE INTO t (...) VALUES (...)
	// Postgres: INSERT INTO t (...) VALUES (...) ON CONFLICT DO NOTHING
	// Strip leading SQL line comments (-- ...) to find the actual statement start.
	trimmed := stripLeadingSQLComments(stmt)
	upper := strings.ToUpper(strings.TrimSpace(trimmed))
	if strings.HasPrefix(upper, "INSERT OR IGNORE INTO") {
		stmt = replaceCaseInsensitivePrefix(trimmed, "INSERT OR IGNORE INTO", "INSERT INTO")
		// Append ON CONFLICT DO NOTHING if not already present.
		if !strings.Contains(strings.ToUpper(stmt), "ON CONFLICT") {
			stmt = strings.TrimSpace(stmt) + " ON CONFLICT DO NOTHING"
		}
	}
	return stmt
}

// stripLeadingSQLComments removes leading SQL line comments (-- to end of line)
// from stmt and returns the result. Comments inside the statement body are
// preserved.
func stripLeadingSQLComments(stmt string) string {
	for {
		s := strings.TrimLeft(stmt, " \t\n\r")
		if len(s) < 2 || s[0] != '-' || s[1] != '-' {
			return s
		}
		// Find end of line.
		end := strings.IndexByte(s, '\n')
		if end < 0 {
			return ""
		}
		stmt = s[end+1:]
	}
}

// replaceCaseInsensitive replaces all case-insensitive occurrences of old with
// new in s. The replacement is literal (no regex).
func replaceCaseInsensitive(s, old, new string) string {
	if old == "" {
		return s
	}
	lower := strings.ToLower(s)
	oldLower := strings.ToLower(old)
	var b strings.Builder
	idx := 0
	for {
		pos := strings.Index(lower[idx:], oldLower)
		if pos < 0 {
			b.WriteString(s[idx:])
			break
		}
		b.WriteString(s[idx : idx+pos])
		b.WriteString(new)
		idx += pos + len(old)
	}
	return b.String()
}

// replaceCaseInsensitivePrefix replaces the first case-insensitive occurrence
// of old at the beginning of s with new. If s does not start with old (case
// insensitive), s is returned unchanged.
func replaceCaseInsensitivePrefix(s, old, new string) string {
	if old == "" {
		return s
	}
	trimmed := strings.TrimLeft(s, " \t\n\r")
	lower := strings.ToLower(trimmed)
	oldLower := strings.ToLower(old)
	if !strings.HasPrefix(lower, oldLower) {
		return s
	}
	// Preserve leading whitespace.
	leading := s[:len(s)-len(trimmed)]
	return leading + new + trimmed[len(old):]
}
