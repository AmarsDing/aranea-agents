package data

import (
	"reflect"
	"testing"
)

// splitDDLStatements must ignore semicolons inside `--` line comments.
// Regression test for the fresh-Postgres failure of migration
// 20261128_memory_facts_fts_index.sql, whose header comment contains
// "Postgres dialect; SQLite CLI/tests skip it." — the naive semicolon split
// produced a statement starting with "SQLite CLI/tests skip it." and Postgres
// rejected it with a syntax error, failing the P1 startup step on clean DBs.
func TestSplitDDLStatements_SemicolonInComment(t *testing.T) {
	ddl := `-- header comment; with semicolon
-- another comment line
CREATE INDEX IF NOT EXISTS idx_t ON t (a);
-- trailing comment; also with semicolon
ALTER TABLE t ADD COLUMN b TEXT NOT NULL DEFAULT '';`
	got := splitDDLStatements(ddl)
	want := []string{
		"CREATE INDEX IF NOT EXISTS idx_t ON t (a)",
		"ALTER TABLE t ADD COLUMN b TEXT NOT NULL DEFAULT ''",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestSplitDDLStatements_PlainMultiStatement(t *testing.T) {
	got := splitDDLStatements("CREATE TABLE a (id TEXT);\nCREATE TABLE b (id TEXT);")
	want := []string{"CREATE TABLE a (id TEXT)", "CREATE TABLE b (id TEXT)"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestSplitDDLStatements_TrailingCommentWithSemicolon(t *testing.T) {
	got := splitDDLStatements("CREATE TABLE a (id TEXT); -- creates a; idempotent\n")
	want := []string{"CREATE TABLE a (id TEXT)"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// splitDDLStatements must not fragment on semicolons or `--` inside
// single-quoted string literals. Regression test for migration
// 20261263_tool_param_rules_seed.sql, whose regex char class `[;&|...]`
// contains a literal semicolon — the naive split truncated the INSERT into
// broken fragments, failing startup migration on fresh DBs.
func TestSplitDDLStatements_SemicolonInStringLiteral(t *testing.T) {
	ddl := `INSERT INTO t (id, pattern) VALUES
  ('a', 're:(?i)(^|[;&|/\s"''])rm\s+-rf'),
  ('b', 'x;y'); -- tail; comment
UPDATE t SET pattern = 'p;q' WHERE id = 'a';`
	want := []string{
		`INSERT INTO t (id, pattern) VALUES
  ('a', 're:(?i)(^|[;&|/\s"''])rm\s+-rf'),
  ('b', 'x;y')`,
		`UPDATE t SET pattern = 'p;q' WHERE id = 'a'`,
	}
	got := splitDDLStatements(ddl)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}
