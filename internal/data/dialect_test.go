package data

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"entgo.io/ent/dialect"
	_ "github.com/glebarez/go-sqlite/compat"
)

func TestDialect_Constants(t *testing.T) {
	if !DialectSQLite.IsSQLite() {
		t.Errorf("DialectSQLite.IsSQLite() = false, want true")
	}
	if DialectSQLite.IsPostgres() {
		t.Errorf("DialectSQLite.IsPostgres() = true, want false")
	}
	if !DialectPostgres.IsPostgres() {
		t.Errorf("DialectPostgres.IsPostgres() = false, want true")
	}
	if DialectPostgres.IsSQLite() {
		t.Errorf("DialectPostgres.IsSQLite() = true, want false")
	}
	if DialectSQLite.String() != "sqlite" {
		t.Errorf("DialectSQLite.String() = %q, want %q", DialectSQLite.String(), "sqlite")
	}
	if DialectPostgres.String() != "postgres" {
		t.Errorf("DialectPostgres.String() = %q, want %q", DialectPostgres.String(), "postgres")
	}
}

func TestDialect_JSONExtract(t *testing.T) {
	tests := []struct {
		dialect Dialect
		col     string
		key     string
		want    string
	}{
		{DialectSQLite, "metadata", "session_id", "json_extract(metadata, '$.session_id')"},
		{DialectPostgres, "metadata", "session_id", "metadata ->> 'session_id'"},
	}
	for _, tt := range tests {
		got := tt.dialect.JSONExtract(tt.col, tt.key)
		if got != tt.want {
			t.Errorf("%s.JSONExtract(%q, %q) = %q, want %q", tt.dialect, tt.col, tt.key, got, tt.want)
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
		{DialectSQLite, "data", []string{"meta", "session_id"}, "json_extract(data, '$.meta.session_id')"},
		{DialectPostgres, "data", []string{"meta", "session_id"}, "data #>> '{meta,session_id}'"},
		{DialectSQLite, "data", []string{}, "data"},
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
		{DialectSQLite, 1, "?"},
		{DialectSQLite, 3, "?,?,?"},
		{DialectPostgres, 1, "$1"},
		{DialectPostgres, 3, "$1,$2,$3"},
		{DialectSQLite, 0, ""},
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
		{DialectSQLite, []string{"1", "x"}, "MAX(1, x)"},
		{DialectPostgres, []string{"1", "x"}, "GREATEST(1, x)"},
		{DialectSQLite, []string{"a", "b", "c"}, "MAX(a, b, c)"},
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
			DialectSQLite, "users", "id, name", "?, ?", "id",
			"INSERT OR IGNORE INTO users (id, name) VALUES (?, ?)",
		},
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

	sqliteQuery, sqliteArgs := DialectSQLite.TableExistsQuery("users")
	if sqliteArgs[0] != "users" {
		t.Errorf("SQLite TableExistsQuery args = %v, want [users]", sqliteArgs)
	}
	if sqliteQuery == "" {
		t.Error("SQLite TableExistsQuery returned empty query")
	}
}

func TestDialect_TableExists_SQLite(t *testing.T) {
	db, err := sql.Open(dialect.SQLite, "file:enttest_dialect?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	if _, err := db.ExecContext(ctx, "CREATE TABLE test_table (id INTEGER PRIMARY KEY)"); err != nil {
		t.Fatalf("create table: %v", err)
	}

	exists, err := DialectSQLite.TableExists(ctx, db, "test_table")
	if err != nil {
		t.Fatalf("TableExists: %v", err)
	}
	if !exists {
		t.Error("TableExists(test_table) = false, want true")
	}

	exists, err = DialectSQLite.TableExists(ctx, db, "nonexistent_table")
	if err != nil {
		t.Fatalf("TableExists: %v", err)
	}
	if exists {
		t.Error("TableExists(nonexistent_table) = true, want false")
	}
}

func TestDialect_ColumnExists_SQLite(t *testing.T) {
	db, err := sql.Open(dialect.SQLite, "file:enttest_dialect_col?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	if _, err := db.ExecContext(ctx, "CREATE TABLE test_table (id INTEGER PRIMARY KEY, name TEXT)"); err != nil {
		t.Fatalf("create table: %v", err)
	}

	exists, err := DialectSQLite.ColumnExists(ctx, db, "test_table", "name")
	if err != nil {
		t.Fatalf("ColumnExists: %v", err)
	}
	if !exists {
		t.Error("ColumnExists(name) = false, want true")
	}

	exists, err = DialectSQLite.ColumnExists(ctx, db, "test_table", "nonexistent")
	if err != nil {
		t.Fatalf("ColumnExists: %v", err)
	}
	if exists {
		t.Error("ColumnExists(nonexistent) = true, want false")
	}
}

func TestDialect_AlreadyExistsErr(t *testing.T) {
	// SQLite "already exists" detection — simulate error message
	sqliteErr := errors.New("table users already exists")
	if !DialectSQLite.AlreadyExistsErr(sqliteErr) {
		t.Error("SQLite AlreadyExistsErr should detect 'already exists'")
	}

	// "duplicate column name" detection
	dupColErr := errors.New("duplicate column name: links")
	if !DialectSQLite.AlreadyExistsErr(dupColErr) {
		t.Error("SQLite AlreadyExistsErr should detect 'duplicate column name'")
	}

	// Non-matching error
	otherErr := errors.New("syntax error")
	if DialectSQLite.AlreadyExistsErr(otherErr) {
		t.Error("SQLite AlreadyExistsErr should not detect 'syntax error'")
	}

	// Nil error
	if DialectSQLite.AlreadyExistsErr(nil) {
		t.Error("AlreadyExistsErr(nil) should be false")
	}
	if DialectPostgres.AlreadyExistsErr(nil) {
		t.Error("AlreadyExistsErr(nil) should be false")
	}
}

func TestData_Dialect(t *testing.T) {
	d := &Data{dialect: DialectPostgres}
	if d.Dialect() != DialectPostgres {
		t.Errorf("Dialect() = %q, want %q", d.Dialect(), DialectPostgres)
	}

	d = &Data{dialect: DialectSQLite}
	if d.Dialect() != DialectSQLite {
		t.Errorf("Dialect() = %q, want %q", d.Dialect(), DialectSQLite)
	}

	// Nil safety
	var nilData *Data
	if nilData.Dialect() != DialectSQLite {
		t.Errorf("nil Data Dialect() = %q, want %q (default)", nilData.Dialect(), DialectSQLite)
	}
}
