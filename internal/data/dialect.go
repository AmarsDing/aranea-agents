package data

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/lib/pq"
)

// Dialect identifies the active SQL dialect for the primary database.
type Dialect string

const (
	// DialectSQLite is the SQLite dialect (used by CLI tools and tests only).
	DialectSQLite Dialect = "sqlite"
	// DialectPostgres is the Postgres dialect (primary DB in production).
	DialectPostgres Dialect = "postgres"
)

// IsSQLite reports whether the dialect is SQLite.
func (d Dialect) IsSQLite() bool { return d == DialectSQLite }

// IsPostgres reports whether the dialect is Postgres.
func (d Dialect) IsPostgres() bool { return d == DialectPostgres }

// String returns the dialect name.
func (d Dialect) String() string { return string(d) }

// JSONExtract returns a SQL expression that extracts a JSON object key as text.
// SQLite: json_extract(col, '$.key')
// Postgres: COALESCE(NULLIF(col::text, ”)::jsonb, '{}'::jsonb) ->> 'key'
//
// The col::text + ::jsonb cast pair is required because cross-dialect JSON
// columns exist in BOTH physical types after the SQLite→Postgres migration:
// raw-SQL-managed columns (monitor_events.metadata_json, sessions state_json)
// are TEXT, while ent field.JSON columns (skill_invocation.token_usage) are
// native jsonb. col::text is the identity on TEXT and serializes jsonb to its
// canonical text form, so one expression serves both. COALESCE/NULLIF guards
// NULL and empty-string rows.
func (d Dialect) JSONExtract(col, key string) string {
	if d.IsPostgres() {
		return fmt.Sprintf("COALESCE(NULLIF(%s::text, '')::jsonb, '{}'::jsonb) ->> '%s'", col, key)
	}
	return fmt.Sprintf("json_extract(%s, '$.%s')", col, key)
}

// JSONExtractNumeric returns a SQL expression that extracts a JSON object key
// as a numeric (DOUBLE PRECISION) value, for use in numeric contexts:
// COALESCE with a DOUBLE PRECISION column, AVG/SUM, and `>` comparisons.
//
// SQLite:   json_extract(col, '$.key')   — returns INTEGER/REAL natively
// Postgres: CAST(NULLIF(col::jsonb ->> 'key', ”) AS DOUBLE PRECISION)
//
// Postgres' `->>` always yields text; without the cast, numeric consumers
// fail with 42804 ("COALESCE types double precision and text cannot be
// matched") or 42883 ("function avg(text) does not exist"). NULLIF(..., ”)
// guards empty-string values which would otherwise fail the cast; missing
// keys and NULL rows yield NULL (propagating through COALESCE/AVG as NULL).
// Non-numeric junk values raise 22P02 — acceptable because writers store
// numeric JSON (same strictness as the meta_duration_ms generated column).
// The col::text base handles both TEXT and native jsonb columns (see
// JSONExtract doc).
func (d Dialect) JSONExtractNumeric(col, key string) string {
	if d.IsPostgres() {
		return fmt.Sprintf("CAST(NULLIF(COALESCE(NULLIF(%s::text, '')::jsonb, '{}'::jsonb) ->> '%s', '') AS DOUBLE PRECISION)", col, key)
	}
	return fmt.Sprintf("json_extract(%s, '$.%s')", col, key)
}

// JSONExtractPath returns a SQL expression that extracts a nested JSON path as text.
// path is a slice of keys, e.g. ["metadata", "session_id"].
// SQLite: json_extract(col, '$.metadata.session_id')
// Postgres: col #>> '{metadata,session_id}'
func (d Dialect) JSONExtractPath(col string, path ...string) string {
	if len(path) == 0 {
		return col
	}
	if d.IsPostgres() {
		return fmt.Sprintf("%s #>> '{%s}'", col, strings.Join(path, ","))
	}
	return fmt.Sprintf("json_extract(%s, '$.%s')", col, strings.Join(path, "."))
}

// JSONEach returns a SQL expression for iterating JSON array elements.
// SQLite: json_each(col)
// Postgres: json_array_elements_text(col)
//
// Usage note: the returned expression should be used in the FROM clause.
// For Postgres, the element column is named "value" by default; for SQLite
// json_each, use "value" as well (both expose a "value" column for text elements).
func (d Dialect) JSONEach(col string) string {
	if d.IsPostgres() {
		return fmt.Sprintf("json_array_elements_text(%s) AS value", col)
	}
	return fmt.Sprintf("json_each(%s)", col)
}

// JSONBBase returns the canonical base expression for JSON write helpers.
// Cross-dialect JSON columns are physically TEXT (raw-SQL-managed) or jsonb
// (ent field.JSON) after the SQLite→Postgres migration, so the Postgres write
// helpers (JSONSet/JSONSetMulti/JSONRemove/JSONRemoveMulti) require their
// input expression to already be jsonb-typed. JSONBBase normalizes either
// physical type into such an expression:
//
// SQLite:   COALESCE(col, '{}')
// Postgres: COALESCE(NULLIF(col::text, ”)::jsonb, '{}'::jsonb)
//
// col::text is the identity on TEXT columns and serializes native jsonb to
// its canonical text form, so both physical types are accepted. The
// COALESCE/NULLIF guard tolerates NULL and empty-string rows (both make
// jsonb_set return NULL or fail the ::jsonb cast). This mirrors the read-side
// convention in JSONExtract.
//
// The resulting expression yields jsonb on Postgres; assigning it back to a
// TEXT column in UPDATE ... SET relies on Postgres' assignment cast
// (jsonb→text CoerceViaIO), verified by TestDialectIntegration; assigning to
// a native jsonb column is a direct jsonb→jsonb assignment.
func (d Dialect) JSONBBase(col string) string {
	if d.IsPostgres() {
		return fmt.Sprintf("COALESCE(NULLIF(%s::text, '')::jsonb, '{}'::jsonb)", col)
	}
	return fmt.Sprintf("COALESCE(%s, '{}')", col)
}

// JSONSet returns a SQL expression that sets a JSON key to a new value.
// SQLite: json_set(col, '$.key', new_value)
// Postgres: jsonb_set(col, '{key}', to_jsonb(new_value::text))
//
// For Postgres, the new_value must be wrapped in to_jsonb() to ensure JSON
// compatibility. The ::text cast is required because to_jsonb(anyelement)
// is polymorphic — without an explicit cast, Postgres cannot infer the
// placeholder type and errors with 42804 ("could not determine polymorphic
// type because input type is unknown"). All current callers pass string
// values (map[string]string or text columns), so ::text is semantically
// correct.
//
// Contract: on Postgres, col MUST be a jsonb-typed expression (produced by
// JSONBBase or a previous jsonb_set call). Passing a bare TEXT column raises
// 42883 ("function jsonb_set(text, unknown, jsonb) does not exist") — the
// historical root cause of run.persist_failed. The guard is applied once at
// the base (JSONBBase) rather than re-applied here, so chained calls stay
// cheap.
//
// Note: for multiple key updates, Postgres requires nested jsonb_set calls.
// Use JSONSetMulti for multi-key updates.
func (d Dialect) JSONSet(col, key, newValue string) string {
	if d.IsPostgres() {
		return fmt.Sprintf("jsonb_set(%s, '{%s}', to_jsonb(%s::text))", col, key, newValue)
	}
	return fmt.Sprintf("json_set(%s, '$.%s', %s)", col, key, newValue)
}

// JSONSetMulti returns a SQL expression that sets multiple JSON keys in a single
// expression. For SQLite, this uses a single json_set call with multiple path/value
// pairs. For Postgres, this nests jsonb_set calls since Postgres jsonb_set only
// supports one path at a time.
//
// pairs is a slice of (key, value) tuples where value is a placeholder or expression.
// Contract: same as JSONSet — on Postgres, col must be jsonb-typed (use JSONBBase).
//
// Example (SQLite): json_set(col, '$.k1', v1, '$.k2', v2)
// Example (Postgres): jsonb_set(jsonb_set(base, '{k1}', to_jsonb(v1::text)), '{k2}', to_jsonb(v2::text))
func (d Dialect) JSONSetMulti(col string, pairs ...[2]string) string {
	if len(pairs) == 0 {
		return col
	}
	if d.IsPostgres() {
		result := col
		for _, p := range pairs {
			result = fmt.Sprintf("jsonb_set(%s, '{%s}', to_jsonb(%s::text))", result, p[0], p[1])
		}
		return result
	}
	parts := make([]string, 0, len(pairs)*2)
	for _, p := range pairs {
		parts = append(parts, fmt.Sprintf("'$.%s'", p[0]), p[1])
	}
	return fmt.Sprintf("json_set(%s, %s)", col, strings.Join(parts, ", "))
}

// JSONRemove returns a SQL expression that removes a top-level key from a JSON
// column, plus the argument to pass for the key placeholder.
//
// SQLite: json_remove(expr, '$.key')  — arg = "$.key"
// Postgres: expr - ?::text            — arg = "key"
//
// Postgres uses the jsonb `-` operator (delete top-level key by text name).
// The ::text cast is required for the same polymorphic-type reason as JSONSet.
// Contract: same as JSONSet — on Postgres, expr must be jsonb-typed (use
// JSONBBase); a bare TEXT column raises 42883 ("operator does not exist:
// text - text").
// Note: only top-level keys are supported; for nested paths use #- with a
// text[] literal (not a placeholder).
func (d Dialect) JSONRemove(expr, key string) (sqlExpr, arg string) {
	if d.IsPostgres() {
		return fmt.Sprintf("%s - ?::text", expr), key
	}
	return fmt.Sprintf("json_remove(%s, ?)", expr), "$." + key
}

// JSONRemoveMulti returns a SQL expression that removes multiple top-level
// keys in one expression, for developer-supplied constant key sets.
//
// SQLite:   json_remove(expr, '$.k1', '$.k2')
// Postgres: expr - '{k1,k2}'::text[]
//
// Keys are embedded as SQL literals (consistent with JSONSet embedding keys
// into '{key}'); they MUST be code constants, never user input. Contract:
// same as JSONSet — on Postgres, expr must be jsonb-typed (use JSONBBase).
func (d Dialect) JSONRemoveMulti(expr string, keys ...string) string {
	if len(keys) == 0 {
		return expr
	}
	if d.IsPostgres() {
		return fmt.Sprintf("%s - '{%s}'::text[]", expr, strings.Join(keys, ","))
	}
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("'$.%s'", k))
	}
	return fmt.Sprintf("json_remove(%s, %s)", expr, strings.Join(parts, ", "))
}

// TableExistsQuery returns the SQL query and args to check if a table exists.
// SQLite: SELECT name FROM sqlite_master WHERE type='table' AND name=?
// Postgres: SELECT table_name FROM information_schema.tables WHERE table_schema='public' AND table_name=$1
func (d Dialect) TableExistsQuery(table string) (string, []any) {
	if d.IsPostgres() {
		return "SELECT table_name FROM information_schema.tables WHERE table_schema = 'public' AND table_name = $1", []any{table}
	}
	return "SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?", []any{table}
}

// ColumnExistsQuery returns the SQL query and args to check if a column exists.
// SQLite: SELECT COUNT(*) FROM pragma_table_info(table) WHERE name=?
// Postgres: SELECT column_name FROM information_schema.columns WHERE table_schema='public' AND table_name=$1 AND column_name=$2
func (d Dialect) ColumnExistsQuery(table, column string) (string, []any) {
	if d.IsPostgres() {
		return "SELECT column_name FROM information_schema.columns WHERE table_schema = 'public' AND table_name = $1 AND column_name = $2", []any{table, column}
	}
	return fmt.Sprintf("SELECT COUNT(*) FROM pragma_table_info('%s') WHERE name = ?", table), []any{column}
}

// IndexExistsQuery returns the SQL query and args to check if an index exists.
// SQLite: SELECT 1 FROM pragma_index_list(table) WHERE name = ? LIMIT 1
// Postgres: SELECT index_name FROM pg_indexes WHERE schemaname = 'public' AND tablename = $1 AND indexname = $2 LIMIT 1
func (d Dialect) IndexExistsQuery(table, indexName string) (string, []any) {
	if d.IsPostgres() {
		return "SELECT index_name FROM pg_indexes WHERE schemaname = 'public' AND tablename = $1 AND indexname = $2 LIMIT 1", []any{table, indexName}
	}
	return fmt.Sprintf("SELECT 1 FROM pragma_index_list('%s') WHERE name = ? LIMIT 1", table), []any{indexName}
}

// InsertOrIgnore returns the SQL prefix for an INSERT that ignores conflicts.
// SQLite: INSERT OR IGNORE INTO ...
// Postgres: INSERT INTO ... ON CONFLICT DO NOTHING
//
// For Postgres, the caller must append "ON CONFLICT DO NOTHING" after the
// VALUES clause. This helper returns just the INSERT keyword prefix for SQLite
// and the standard INSERT for Postgres; use BuildInsertOrIgnore for a
// complete statement.
func (d Dialect) InsertOrIgnore() string {
	if d.IsPostgres() {
		return "INSERT"
	}
	return "INSERT OR IGNORE"
}

// BuildInsertOrIgnore builds a complete INSERT statement that ignores conflicts.
// columns is the comma-separated column list, placeholders is the comma-separated
// placeholder list (e.g. "?,?,?" or "$1,$2,$3"), conflictCols is the
// comma-separated column list for the ON CONFLICT clause (Postgres only).
//
// Example (SQLite): INSERT OR IGNORE INTO users (id, name) VALUES (?, ?)
// Example (Postgres): INSERT INTO users (id, name) VALUES ($1, $2) ON CONFLICT (id) DO NOTHING
func (d Dialect) BuildInsertOrIgnore(table, columns, placeholders, conflictCols string) string {
	if d.IsPostgres() {
		return fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s) ON CONFLICT (%s) DO NOTHING", table, columns, placeholders, conflictCols)
	}
	return fmt.Sprintf("INSERT OR IGNORE INTO %s (%s) VALUES (%s)", table, columns, placeholders)
}

// Placeholder returns the positional placeholder for the given 1-based index.
// SQLite: ?
// Postgres: $N
func (d Dialect) Placeholder(index int) string {
	if d.IsPostgres() {
		return fmt.Sprintf("$%d", index)
	}
	return "?"
}

// Greatest returns the SQL scalar function that picks the largest of the given
// arguments. SQLite uses the multi-argument MAX (a scalar function when given
// 2+ args), while Postgres uses GREATEST. Postgres' MAX is aggregate-only and
// rejects multi-argument scalar usage (error 42883).
//
// Example: DialectPostgres.Greatest("1", "x") → "GREATEST(1, x)"
//
// All major SQLite drivers support multi-argument MAX as a scalar function.
func (d Dialect) Greatest(args ...string) string {
	fn := "GREATEST"
	if d.IsSQLite() {
		fn = "MAX"
	}
	return fn + "(" + strings.Join(args, ", ") + ")"
}

// Placeholders returns n positional placeholders joined by commas.
// SQLite: ?,?,?
// Postgres: $1,$2,$3
func (d Dialect) Placeholders(n int) string {
	if n <= 0 {
		return ""
	}
	if d.IsPostgres() {
		parts := make([]string, n)
		for i := 0; i < n; i++ {
			parts[i] = fmt.Sprintf("$%d", i+1)
		}
		return strings.Join(parts, ",")
	}
	return strings.Repeat("?,", n-1) + "?"
}

// RenumberPlaceholders converts SQLite-style ? placeholders in a SQL fragment
// to Postgres-style $N placeholders, starting from index 1. For SQLite dialect,
// the input is returned unchanged.
//
// Example (Postgres): "WHERE a = ? AND b = ?" → "WHERE a = $1 AND b = $2"
//
// Note: this function does NOT handle escaped ?? sequences or ? inside string
// literals. Use it only for SQL fragments where ? is exclusively a placeholder.
func (d Dialect) RenumberPlaceholders(sql string) string {
	if d.IsSQLite() {
		return sql
	}
	var b strings.Builder
	idx := 1
	for i := 0; i < len(sql); i++ {
		if sql[i] == '?' {
			b.WriteString(fmt.Sprintf("$%d", idx))
			idx++
			continue
		}
		b.WriteByte(sql[i])
	}
	return b.String()
}

// AlreadyExistsErr reports whether err indicates a "duplicate/already exists"
// condition for the given dialect. Used for idempotent DDL migrations.
func (d Dialect) AlreadyExistsErr(err error) bool {
	if err == nil {
		return false
	}
	if d.IsPostgres() {
		var pgErr *pq.Error
		if errors.As(err, &pgErr) {
			// 42P07 = duplicate_table, 42710 = duplicate_object, 42701 = duplicate_column
			switch pgErr.Code {
			case "42P07", "42710", "42701":
				return true
			}
		}
		return false
	}
	// SQLite: "already exists" or "duplicate column"
	msg := err.Error()
	return strings.Contains(msg, "already exists") || strings.Contains(msg, "duplicate column name")
}

// UndefinedObjectErr reports whether err indicates a "no such table/column/object"
// condition for the given dialect. Used for idempotent DDL migrations where a
// later migration will create the missing object.
func (d Dialect) UndefinedObjectErr(err error) bool {
	if err == nil {
		return false
	}
	if d.IsPostgres() {
		var pgErr *pq.Error
		if errors.As(err, &pgErr) {
			// 42P01 = undefined_table, 42703 = undefined_column
			switch pgErr.Code {
			case "42P01", "42703":
				return true
			}
		}
		return false
	}
	// SQLite: "no such table" / "no such column"
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "no such table") || strings.Contains(msg, "no such column")
}

// UniqueConstraintErr reports whether err indicates a unique constraint
// violation (duplicate key) for the given dialect. Used for idempotent
// INSERT operations that skip duplicates.
func (d Dialect) UniqueConstraintErr(err error) bool {
	if err == nil {
		return false
	}
	if d.IsPostgres() {
		var pgErr *pq.Error
		if errors.As(err, &pgErr) {
			// 23505 = unique_violation
			return pgErr.Code == "23505"
		}
		return false
	}
	// SQLite: "UNIQUE constraint failed"
	msg := err.Error()
	return strings.Contains(msg, "UNIQUE constraint failed") || strings.Contains(msg, "unique constraint")
}

// TableExists checks whether a table exists in the database.
func (d Dialect) TableExists(ctx context.Context, db *sql.DB, table string) (bool, error) {
	q, args := d.TableExistsQuery(table)
	var name string
	err := db.QueryRowContext(ctx, q, args...).Scan(&name)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return name != "", nil
}

// ColumnExists checks whether a column exists in the given table.
func (d Dialect) ColumnExists(ctx context.Context, db *sql.DB, table, column string) (bool, error) {
	q, args := d.ColumnExistsQuery(table, column)
	if d.IsPostgres() {
		var name string
		err := db.QueryRowContext(ctx, q, args...).Scan(&name)
		if err == sql.ErrNoRows {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		return name != "", nil
	}
	// SQLite: COUNT(*)
	var count int
	if err := db.QueryRowContext(ctx, q, args...).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

// IndexExists checks whether an index exists on the given table.
func (d Dialect) IndexExists(ctx context.Context, db *sql.DB, table, indexName string) (bool, error) {
	q, args := d.IndexExistsQuery(table, indexName)
	var name string
	err := db.QueryRowContext(ctx, q, args...).Scan(&name)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return name != "", nil
}
