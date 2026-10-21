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
//
// Post-migration (2026-10-21): SQLite support has been removed entirely.
// The only supported dialect is Postgres (NewData hardcodes it). The
// Dialect type and helpers are kept so the raw-SQL call sites that build
// dialect-aware SQL compile unchanged; every helper now returns the
// Postgres form unconditionally.
type Dialect string

const (
	// DialectPostgres is the Postgres dialect (primary DB in production,
	// the only supported dialect).
	DialectPostgres Dialect = "postgres"
)

// IsPostgres reports whether the dialect is Postgres. Kept for call-site
// compatibility; always true for any Dialect produced by this package.
func (d Dialect) IsPostgres() bool { return d == DialectPostgres || d == "" }

// String returns the dialect name.
func (d Dialect) String() string { return string(d) }

// JSONExtract returns a Postgres SQL expression that extracts a JSON object
// key as text:
//
//	COALESCE(NULLIF(col::text, '')::jsonb, '{}'::jsonb) ->> 'key'
//
// The col::text + ::jsonb cast pair is required because JSON columns exist
// in BOTH physical types after the SQLite→Postgres migration: raw-SQL-managed
// columns (monitor_events.metadata_json, sessions state_json) are TEXT, while
// ent field.JSON columns (skill_invocation.token_usage) are native jsonb.
// col::text is the identity on TEXT and serializes jsonb to its canonical
// text form, so one expression serves both. COALESCE/NULLIF guards NULL and
// empty-string rows.
func (d Dialect) JSONExtract(col, key string) string {
	return fmt.Sprintf("COALESCE(NULLIF(%s::text, '')::jsonb, '{}'::jsonb) ->> '%s'", col, key)
}

// JSONExtractNumeric returns a Postgres SQL expression that extracts a JSON
// object key as a numeric (DOUBLE PRECISION) value, for use in numeric
// contexts: COALESCE with a DOUBLE PRECISION column, AVG/SUM, and `>`
// comparisons:
//
//	CAST(NULLIF(COALESCE(NULLIF(col::text, '')::jsonb, '{}'::jsonb) ->> 'key', '') AS DOUBLE PRECISION)
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
	return fmt.Sprintf("CAST(NULLIF(COALESCE(NULLIF(%s::text, '')::jsonb, '{}'::jsonb) ->> '%s', '') AS DOUBLE PRECISION)", col, key)
}

// JSONExtractPath returns a Postgres SQL expression that extracts a nested
// JSON path as text. path is a slice of keys, e.g. ["metadata", "session_id"]:
//
//	col #>> '{metadata,session_id}'
func (d Dialect) JSONExtractPath(col string, path ...string) string {
	if len(path) == 0 {
		return col
	}
	// 列可能声明为 TEXT（如 unified_evolution_suggestions.metadata），
	// #>> 要求 jsonb 操作数，复用 JSONBBase 的 TEXT→jsonb 归一化。
	return fmt.Sprintf("%s #>> '{%s}'", d.JSONBBase(col), strings.Join(path, ","))
}

// JSONEach returns a Postgres SQL expression for iterating JSON array
// elements:
//
//	json_array_elements_text(COALESCE(NULLIF(col::text, '')::json, '[]'::json)) AS value
//
// Usage note: the returned expression should be used in the FROM clause;
// the element column is named "value" by default.
//
// The col::text + ::json normalization mirrors JSONBBase: JSON columns exist
// in both physical types after the SQLite→Postgres migration (raw-SQL-managed
// TEXT and ent field.JSON native jsonb), and json_array_elements_text has no
// jsonb overload (the jsonb variant is jsonb_array_elements_text) — a bare
// jsonb operand raises 42883 ("function json_array_elements_text(jsonb) does
// not exist"). NULL/missing values coalesce to '[]' (zero rows, same as the
// strict-function behavior on NULL).
func (d Dialect) JSONEach(col string) string {
	return fmt.Sprintf("json_array_elements_text(COALESCE(NULLIF(%s::text, '')::json, '[]'::json)) AS value", col)
}

// JSONBBase returns the canonical base expression for JSON write helpers.
// JSON columns are physically TEXT (raw-SQL-managed) or jsonb (ent
// field.JSON) after the SQLite→Postgres migration, so the Postgres write
// helpers (JSONSet/JSONSetMulti/JSONRemove/JSONRemoveMulti) require their
// input expression to already be jsonb-typed. JSONBBase normalizes either
// physical type into such an expression:
//
//	COALESCE(NULLIF(col::text, '')::jsonb, '{}'::jsonb)
//
// col::text is the identity on TEXT columns and serializes native jsonb to
// its canonical text form, so both physical types are accepted. The
// COALESCE/NULLIF guard tolerates NULL and empty-string rows (both make
// jsonb_set return NULL or fail the ::jsonb cast). This mirrors the read-side
// convention in JSONExtract.
//
// The resulting expression yields jsonb; assigning it back to a TEXT column
// in UPDATE ... SET relies on Postgres' assignment cast (jsonb→text
// CoerceViaIO), verified by TestDialectIntegration; assigning to a native
// jsonb column is a direct jsonb→jsonb assignment.
func (d Dialect) JSONBBase(col string) string {
	return fmt.Sprintf("COALESCE(NULLIF(%s::text, '')::jsonb, '{}'::jsonb)", col)
}

// JSONSet returns a Postgres SQL expression that sets a JSON key to a new
// value:
//
//	jsonb_set(col, '{key}', to_jsonb(new_value::text))
//
// The to_jsonb() wrap ensures JSON compatibility. The ::text cast is
// required because to_jsonb(anyelement) is polymorphic — without an explicit
// cast, Postgres cannot infer the placeholder type and errors with 42804
// ("could not determine polymorphic type because input type is unknown").
// All current callers pass string values (map[string]string or text columns),
// so ::text is semantically correct.
//
// Contract: col MUST be a jsonb-typed expression (produced by JSONBBase or a
// previous jsonb_set call). Passing a bare TEXT column raises 42883
// ("function jsonb_set(text, unknown, jsonb) does not exist") — the
// historical root cause of run.persist_failed. The guard is applied once at
// the base (JSONBBase) rather than re-applied here, so chained calls stay
// cheap.
//
// Note: for multiple key updates, Postgres requires nested jsonb_set calls.
// Use JSONSetMulti for multi-key updates.
func (d Dialect) JSONSet(col, key, newValue string) string {
	return fmt.Sprintf("jsonb_set(%s, '{%s}', to_jsonb(%s::text))", col, key, newValue)
}

// JSONSetMulti returns a Postgres SQL expression that sets multiple JSON keys
// in a single expression by nesting jsonb_set calls (Postgres jsonb_set only
// supports one path at a time).
//
// pairs is a slice of (key, value) tuples where value is a placeholder or
// expression. Contract: same as JSONSet — col must be jsonb-typed (use
// JSONBBase).
//
// Example: jsonb_set(jsonb_set(base, '{k1}', to_jsonb(v1::text)), '{k2}', to_jsonb(v2::text))
func (d Dialect) JSONSetMulti(col string, pairs ...[2]string) string {
	if len(pairs) == 0 {
		return col
	}
	result := col
	for _, p := range pairs {
		result = fmt.Sprintf("jsonb_set(%s, '{%s}', to_jsonb(%s::text))", result, p[0], p[1])
	}
	return result
}

// JSONRemove returns a Postgres SQL expression that removes a top-level key
// from a JSON column, plus the argument to pass for the key placeholder:
//
//	expr - ?::text            — arg = "key"
//
// Postgres uses the jsonb `-` operator (delete top-level key by text name).
// The ::text cast is required for the same polymorphic-type reason as JSONSet.
// Contract: same as JSONSet — expr must be jsonb-typed (use JSONBBase); a
// bare TEXT column raises 42883 ("operator does not exist: text - text").
// Note: only top-level keys are supported; for nested paths use #- with a
// text[] literal (not a placeholder).
func (d Dialect) JSONRemove(expr, key string) (sqlExpr, arg string) {
	return fmt.Sprintf("%s - ?::text", expr), key
}

// JSONRemoveMulti returns a Postgres SQL expression that removes multiple
// top-level keys in one expression, for developer-supplied constant key sets:
//
//	expr - '{k1,k2}'::text[]
//
// Keys are embedded as SQL literals (consistent with JSONSet embedding keys
// into '{key}'); they MUST be code constants, never user input. Contract:
// same as JSONSet — expr must be jsonb-typed (use JSONBBase).
func (d Dialect) JSONRemoveMulti(expr string, keys ...string) string {
	if len(keys) == 0 {
		return expr
	}
	return fmt.Sprintf("%s - '{%s}'::text[]", expr, strings.Join(keys, ","))
}

// TableExistsQuery returns the SQL query and args to check if a table exists:
//
//	SELECT table_name FROM information_schema.tables WHERE table_schema=current_schema() AND table_name=$1
func (d Dialect) TableExistsQuery(table string) (string, []any) {
	return "SELECT table_name FROM information_schema.tables WHERE table_schema = current_schema() AND table_name = $1", []any{table}
}

// ColumnExistsQuery returns the SQL query and args to check if a column
// exists:
//
//	SELECT column_name FROM information_schema.columns WHERE table_schema=current_schema() AND table_name=$1 AND column_name=$2
func (d Dialect) ColumnExistsQuery(table, column string) (string, []any) {
	return "SELECT column_name FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = $1 AND column_name = $2", []any{table, column}
}

// IndexExistsQuery returns the SQL query and args to check if an index
// exists:
//
//	SELECT indexname FROM pg_indexes WHERE schemaname = current_schema() AND tablename = $1 AND indexname = $2 LIMIT 1
func (d Dialect) IndexExistsQuery(table, indexName string) (string, []any) {
	return "SELECT indexname FROM pg_indexes WHERE schemaname = current_schema() AND tablename = $1 AND indexname = $2 LIMIT 1", []any{table, indexName}
}

// InsertOrIgnore returns the standard INSERT keyword. In Postgres, "ignore
// conflicts" is expressed by appending "ON CONFLICT DO NOTHING" after the
// VALUES clause; use BuildInsertOrIgnore for a complete statement.
func (d Dialect) InsertOrIgnore() string {
	return "INSERT"
}

// BuildInsertOrIgnore builds a complete INSERT statement that ignores
// conflicts. columns is the comma-separated column list, placeholders is the
// comma-separated placeholder list (e.g. "$1,$2,$3"), conflictCols is the
// comma-separated column list for the ON CONFLICT clause.
//
// Example: INSERT INTO users (id, name) VALUES ($1, $2) ON CONFLICT (id) DO NOTHING
func (d Dialect) BuildInsertOrIgnore(table, columns, placeholders, conflictCols string) string {
	return fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s) ON CONFLICT (%s) DO NOTHING", table, columns, placeholders, conflictCols)
}

// Placeholder returns the positional placeholder for the given 1-based
// index: $N.
func (d Dialect) Placeholder(index int) string {
	return fmt.Sprintf("$%d", index)
}

// Greatest returns the SQL scalar function that picks the largest of the
// given arguments. Postgres uses GREATEST; its MAX is aggregate-only and
// rejects multi-argument scalar usage (error 42883).
//
// Example: DialectPostgres.Greatest("1", "x") → "GREATEST(1, x)"
func (d Dialect) Greatest(args ...string) string {
	return "GREATEST(" + strings.Join(args, ", ") + ")"
}

// Least returns the SQL scalar function that picks the smallest of the given
// arguments. Postgres uses LEAST; its MIN is aggregate-only and rejects
// multi-argument scalar usage (error 42883).
//
// Example: DialectPostgres.Least("1", "x") → "LEAST(1, x)"
func (d Dialect) Least(args ...string) string {
	return "LEAST(" + strings.Join(args, ", ") + ")"
}

// Placeholders returns n positional placeholders joined by commas: $1,$2,$3.
func (d Dialect) Placeholders(n int) string {
	if n <= 0 {
		return ""
	}
	parts := make([]string, n)
	for i := 0; i < n; i++ {
		parts[i] = fmt.Sprintf("$%d", i+1)
	}
	return strings.Join(parts, ",")
}

// RenumberPlaceholders converts ? placeholders in a SQL fragment to
// Postgres-style $N placeholders, starting from index 1.
//
// Example: "WHERE a = ? AND b = ?" → "WHERE a = $1 AND b = $2"
//
// Note: this function does NOT handle escaped ?? sequences or ? inside string
// literals. Use it only for SQL fragments where ? is exclusively a placeholder.
func (d Dialect) RenumberPlaceholders(sql string) string {
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
// condition. Used for idempotent DDL migrations.
func (d Dialect) AlreadyExistsErr(err error) bool {
	if err == nil {
		return false
	}
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

// UndefinedObjectErr reports whether err indicates a "no such
// table/column/object" condition. Used for idempotent DDL migrations where a
// later migration will create the missing object.
func (d Dialect) UndefinedObjectErr(err error) bool {
	if err == nil {
		return false
	}
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

// UniqueConstraintErr reports whether err indicates a unique constraint
// violation (duplicate key). Used for idempotent INSERT operations that skip
// duplicates.
func (d Dialect) UniqueConstraintErr(err error) bool {
	if err == nil {
		return false
	}
	var pgErr *pq.Error
	if errors.As(err, &pgErr) {
		// 23505 = unique_violation
		return pgErr.Code == "23505"
	}
	return false
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
