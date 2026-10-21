package data

import (
	"errors"
	"testing"
)

func TestDialect_Constants(t *testing.T) {
	if !DialectPostgres.IsPostgres() {
		t.Errorf("DialectPostgres.IsPostgres() = false, want true")
	}
	if DialectPostgres.String() != "postgres" {
		t.Errorf("DialectPostgres.String() = %q, want %q", DialectPostgres.String(), "postgres")
	}
	// The unset zero-value dialect behaves as Postgres (Data.Dialect() fallback).
	if !(Dialect("")).IsPostgres() {
		t.Errorf("empty dialect IsPostgres() = false, want true (default)")
	}
}

func TestDialect_JSONExtract(t *testing.T) {
	tests := []struct {
		dialect Dialect
		col     string
		key     string
		want    string
	}{
		{DialectPostgres, "metadata", "session_id", "COALESCE(NULLIF(metadata::text, '')::jsonb, '{}'::jsonb) ->> 'session_id'"},
	}
	for _, tt := range tests {
		got := tt.dialect.JSONExtract(tt.col, tt.key)
		if got != tt.want {
			t.Errorf("%s.JSONExtract(%q, %q) = %q, want %q", tt.dialect, tt.col, tt.key, got, tt.want)
		}
	}
}

// TestDialect_JSONExtractNumeric verifies the numeric variant of JSONExtract.
// Postgres `->>` always returns text, so numeric consumers (COALESCE with a
// DOUBLE PRECISION column, AVG, `> 0` comparisons) must cast — otherwise PG
// raises 42804 ("COALESCE types double precision and text cannot be matched")
// or 42883 ("function avg(text) does not exist"). NULLIF(..., ”) guards
// empty-string values which would fail the cast; missing keys yield NULL.
func TestDialect_JSONExtractNumeric(t *testing.T) {
	tests := []struct {
		dialect Dialect
		col     string
		key     string
		want    string
	}{
		{
			DialectPostgres, "metadata_json", "duration_ms",
			"CAST(NULLIF(COALESCE(NULLIF(metadata_json::text, '')::jsonb, '{}'::jsonb) ->> 'duration_ms', '') AS DOUBLE PRECISION)",
		},
		{
			DialectPostgres, "token_usage", "total",
			"CAST(NULLIF(COALESCE(NULLIF(token_usage::text, '')::jsonb, '{}'::jsonb) ->> 'total', '') AS DOUBLE PRECISION)",
		},
	}
	for _, tt := range tests {
		got := tt.dialect.JSONExtractNumeric(tt.col, tt.key)
		if got != tt.want {
			t.Errorf("%s.JSONExtractNumeric(%q, %q) = %q, want %q", tt.dialect, tt.col, tt.key, got, tt.want)
		}
	}
}

// TestDialect_JSONBBase verifies the TEXT→jsonb normalization expression used
// as the base for all JSON write helpers. JSON columns may be TEXT, so
// Postgres write helpers must cast the column to jsonb before applying
// jsonb_set / the jsonb `-` operator — otherwise PG raises 42883 ("function
// jsonb_set(text, unknown, jsonb) does not exist"). The COALESCE(NULLIF(col,
// ”)...) guard tolerates NULL and empty-string rows.
func TestDialect_JSONBBase(t *testing.T) {
	tests := []struct {
		dialect Dialect
		col     string
		want    string
	}{
		{DialectPostgres, "state_json", "COALESCE(NULLIF(state_json::text, '')::jsonb, '{}'::jsonb)"},
		{DialectPostgres, "metadata_json", "COALESCE(NULLIF(metadata_json::text, '')::jsonb, '{}'::jsonb)"},
	}
	for _, tt := range tests {
		if got := tt.dialect.JSONBBase(tt.col); got != tt.want {
			t.Errorf("%s.JSONBBase(%q) = %q, want %q", tt.dialect, tt.col, got, tt.want)
		}
	}
}

// TestDialect_JSONSet verifies the Postgres branch emits the ::text cast on the
// new-value placeholder. Without this cast, to_jsonb(anyelement) fails with
// SQLSTATE 42804 ("could not determine polymorphic type because input type is
// unknown") when the placeholder type is unknown.
//
// Contract: on Postgres the input expr must already be jsonb-typed (produced
// by JSONBBase or a previous jsonb_set). Passing a bare TEXT column is a
// programming error (42883) — this is NOT re-guarded here so nested calls
// stay cheap. See internal/data/dialect.go JSONSet doc comment.
func TestDialect_JSONSet(t *testing.T) {
	pgBase := DialectPostgres.JSONBBase("state_json")
	tests := []struct {
		dialect  Dialect
		col      string
		key      string
		newValue string
		want     string
	}{
		{
			DialectPostgres, pgBase, "run_id", "?",
			"jsonb_set(COALESCE(NULLIF(state_json::text, '')::jsonb, '{}'::jsonb), '{run_id}', to_jsonb(?::text))",
		},
		{
			// chained: input expr is the jsonb output of a previous jsonb_set
			DialectPostgres, "jsonb_set(base, '{k}', to_jsonb(?::text))", "run_id", "?",
			"jsonb_set(jsonb_set(base, '{k}', to_jsonb(?::text)), '{run_id}', to_jsonb(?::text))",
		},
	}
	for _, tt := range tests {
		got := tt.dialect.JSONSet(tt.col, tt.key, tt.newValue)
		if got != tt.want {
			t.Errorf("%s.JSONSet(%q, %q, %q) = %q, want %q", tt.dialect, tt.col, tt.key, tt.newValue, got, tt.want)
		}
	}
}

// TestDialect_JSONSetMulti verifies that:
//   - empty pairs returns the input expr unchanged (no wrapping);
//   - Postgres nests jsonb_set calls with ::text cast on each value placeholder.
func TestDialect_JSONSetMulti(t *testing.T) {
	if got := DialectPostgres.JSONSetMulti("state_json"); got != "state_json" {
		t.Errorf("Postgres JSONSetMulti(empty) = %q, want %q", got, "state_json")
	}

	pgBase := DialectPostgres.JSONBBase("state_json")
	tests := []struct {
		dialect Dialect
		col     string
		pairs   [][2]string
		want    string
	}{
		{
			DialectPostgres, pgBase,
			[][2]string{{"run_id", "?"}},
			"jsonb_set(COALESCE(NULLIF(state_json::text, '')::jsonb, '{}'::jsonb), '{run_id}', to_jsonb(?::text))",
		},
		{
			DialectPostgres, pgBase,
			[][2]string{{"run_id", "?"}, {"status", "?"}},
			"jsonb_set(jsonb_set(COALESCE(NULLIF(state_json::text, '')::jsonb, '{}'::jsonb), '{run_id}', to_jsonb(?::text)), '{status}', to_jsonb(?::text))",
		},
	}
	for _, tt := range tests {
		got := tt.dialect.JSONSetMulti(tt.col, tt.pairs...)
		if got != tt.want {
			t.Errorf("%s.JSONSetMulti(%q, %v) = %q, want %q", tt.dialect, tt.col, tt.pairs, got, tt.want)
		}
	}
}

// TestDialect_JSONRemove verifies the key-removal expression and its
// placeholder arg. Postgres uses the jsonb `-` operator with a ::text cast
// (same polymorphic-type reason as JSONSet). The (sqlExpr, arg) shape lets
// callers thread the arg into their own args slice while embedding the expr
// into a larger UPDATE.
//
// Contract (same as JSONSet): the input expr must be jsonb-typed.
func TestDialect_JSONRemove(t *testing.T) {
	tests := []struct {
		dialect  Dialect
		expr     string
		key      string
		wantExpr string
		wantArg  string
	}{
		{
			DialectPostgres, DialectPostgres.JSONBBase("state_json"), "run_id",
			"COALESCE(NULLIF(state_json::text, '')::jsonb, '{}'::jsonb) - ?::text", "run_id",
		},
		{
			DialectPostgres, "jsonb_set(base, '{k}', to_jsonb(?::text))", "run_id",
			"jsonb_set(base, '{k}', to_jsonb(?::text)) - ?::text", "run_id",
		},
	}
	for _, tt := range tests {
		gotExpr, gotArg := tt.dialect.JSONRemove(tt.expr, tt.key)
		if gotExpr != tt.wantExpr {
			t.Errorf("%s.JSONRemove(%q, %q) expr = %q, want %q", tt.dialect, tt.expr, tt.key, gotExpr, tt.wantExpr)
		}
		if gotArg != tt.wantArg {
			t.Errorf("%s.JSONRemove(%q, %q) arg = %q, want %q", tt.dialect, tt.expr, tt.key, gotArg, tt.wantArg)
		}
	}
}

// TestDialect_JSONRemoveMulti verifies multi-key removal for constant key sets:
//   - empty keys returns the input expr unchanged;
//   - Postgres uses the jsonb `- text[]` operator (keys are developer-supplied
//     constants, embedded as a text[] literal — never user input).
func TestDialect_JSONRemoveMulti(t *testing.T) {
	if got := DialectPostgres.JSONRemoveMulti("state_json"); got != "state_json" {
		t.Errorf("Postgres JSONRemoveMulti(empty) = %q, want %q", got, "state_json")
	}

	tests := []struct {
		dialect Dialect
		expr    string
		keys    []string
		want    string
	}{
		{
			DialectPostgres, DialectPostgres.JSONBBase("metadata_json"),
			[]string{"a", "b"},
			"COALESCE(NULLIF(metadata_json::text, '')::jsonb, '{}'::jsonb) - '{a,b}'::text[]",
		},
	}
	for _, tt := range tests {
		if got := tt.dialect.JSONRemoveMulti(tt.expr, tt.keys...); got != tt.want {
			t.Errorf("%s.JSONRemoveMulti(%q, %v) = %q, want %q", tt.dialect, tt.expr, tt.keys, got, tt.want)
		}
	}
}

func TestDialect_JSONExtractPath(t *testing.T) {
	tests := []struct {
		dialect Dialect
		col     string
		path    []string
		want    string
	}{
		{DialectPostgres, "data", []string{"meta", "session_id"}, "COALESCE(NULLIF(data::text, '')::jsonb, '{}'::jsonb) #>> '{meta,session_id}'"},
		{DialectPostgres, "data", []string{}, "data"},
	}
	for _, tt := range tests {
		got := tt.dialect.JSONExtractPath(tt.col, tt.path...)
		if got != tt.want {
			t.Errorf("%s.JSONExtractPath(%q, %v) = %q, want %q", tt.dialect, tt.col, tt.path, got, tt.want)
		}
	}
}

func TestDialect_Placeholders(t *testing.T) {
	tests := []struct {
		dialect Dialect
		n       int
		want    string
	}{
		{DialectPostgres, 1, "$1"},
		{DialectPostgres, 3, "$1,$2,$3"},
		{DialectPostgres, 0, ""},
	}
	for _, tt := range tests {
		got := tt.dialect.Placeholders(tt.n)
		if got != tt.want {
			t.Errorf("%s.Placeholders(%d) = %q, want %q", tt.dialect, tt.n, got, tt.want)
		}
	}
}

func TestDialect_Greatest(t *testing.T) {
	tests := []struct {
		dialect Dialect
		args    []string
		want    string
	}{
		{DialectPostgres, []string{"1", "x"}, "GREATEST(1, x)"},
		{DialectPostgres, []string{"a", "b", "c"}, "GREATEST(a, b, c)"},
	}
	for _, tt := range tests {
		got := tt.dialect.Greatest(tt.args...)
		if got != tt.want {
			t.Errorf("%s.Greatest(%v) = %q, want %q", tt.dialect, tt.args, got, tt.want)
		}
	}
}

func TestDialect_BuildInsertOrIgnore(t *testing.T) {
	tests := []struct {
		dialect      Dialect
		table        string
		columns      string
		placeholders string
		conflictCols string
		want         string
	}{
		{
			DialectPostgres, "users", "id, name", "$1, $2", "id",
			"INSERT INTO users (id, name) VALUES ($1, $2) ON CONFLICT (id) DO NOTHING",
		},
	}
	for _, tt := range tests {
		got := tt.dialect.BuildInsertOrIgnore(tt.table, tt.columns, tt.placeholders, tt.conflictCols)
		if got != tt.want {
			t.Errorf("%s.BuildInsertOrIgnore(...) = %q, want %q", tt.dialect, got, tt.want)
		}
	}
}

func TestDialect_TableExistsQuery(t *testing.T) {
	pgQuery, pgArgs := DialectPostgres.TableExistsQuery("users")
	if pgArgs[0] != "users" {
		t.Errorf("Postgres TableExistsQuery args = %v, want [users]", pgArgs)
	}
	if pgQuery == "" {
		t.Error("Postgres TableExistsQuery returned empty query")
	}
}

func TestDialect_AlreadyExistsErr(t *testing.T) {
	// Nil error
	if DialectPostgres.AlreadyExistsErr(nil) {
		t.Error("AlreadyExistsErr(nil) should be false")
	}
	// Non-PQ error is never an already-exists condition.
	if DialectPostgres.AlreadyExistsErr(errors.New("syntax error")) {
		t.Error("AlreadyExistsErr should not match non-PQ errors")
	}
}

func TestDialect_UndefinedObjectErr(t *testing.T) {
	// Nil error
	if DialectPostgres.UndefinedObjectErr(nil) {
		t.Error("UndefinedObjectErr(nil) should be false")
	}
	// Non-PQ error is never an undefined-object condition.
	if DialectPostgres.UndefinedObjectErr(errors.New("no such table: foo")) {
		t.Error("UndefinedObjectErr should not match non-PQ errors")
	}
}

func TestData_Dialect(t *testing.T) {
	d := &Data{dialect: DialectPostgres}
	if d.Dialect() != DialectPostgres {
		t.Errorf("Dialect() = %q, want %q", d.Dialect(), DialectPostgres)
	}

	// Nil safety
	var nilData *Data
	if nilData.Dialect() != DialectPostgres {
		t.Errorf("nil Data Dialect() = %q, want %q (default)", nilData.Dialect(), DialectPostgres)
	}
}
