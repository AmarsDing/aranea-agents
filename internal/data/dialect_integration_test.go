package data

import (
	"context"
	"database/sql"
	"os"
	"testing"

	_ "github.com/lib/pq"
)

// dialectIntegrationDB opens a Postgres connection for dialect integration
// tests, or skips the test when ARANEA_TEST_PG_DSN is not set.
//
// Rationale: the dialect JSON helpers are unit-tested by asserting generated
// SQL strings, but that cannot catch type-mismatch errors that only surface
// when Postgres actually parses the statement (e.g. 42804 polymorphic-type
// inference, 42883 "function jsonb_set(text, ...) does not exist"). Those
// bugs shipped twice because SQLite-based tests cannot see them — SQLite
// accepts json_set on TEXT natively. Executing the helpers against a real
// Postgres closes that gap.
//
// Example:
//
//	ARANEA_TEST_PG_DSN="postgres://user:pass@127.0.0.1:5432/db?sslmode=disable" \
//	  go test ./internal/data/ -run TestDialectIntegration -count=1
func dialectIntegrationDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("ARANEA_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("ARANEA_TEST_PG_DSN not set; skipping Postgres dialect integration test")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	db.SetMaxOpenConns(1) // single session: TEMP table must survive across statements
	if err := db.Ping(); err != nil {
		db.Close()
		t.Skipf("postgres unreachable (%v); skipping", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// TestDialectIntegration_JSONWriteHelpers executes the JSONBBase → JSONSet →
// JSONRemove chain in the exact shape PatchSessionState generates, against a
// real Postgres TEXT column. It guards against regression of the
// jsonb_set(text, ...) 42883 root cause and the text - text operator error.
func TestDialectIntegration_JSONWriteHelpers(t *testing.T) {
	db := dialectIntegrationDB(t)
	ctx := context.Background()
	d := DialectPostgres

	mustExec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.ExecContext(ctx, q, args...); err != nil {
			t.Fatalf("exec failed: %v\nSQL: %s", err, q)
		}
	}

	mustExec(`CREATE TEMP TABLE t_dialect_probe (id text PRIMARY KEY, state_json text)`)
	mustExec(`INSERT INTO t_dialect_probe (id, state_json) VALUES ('ok', '{}'), ('empty', ''), ('null', NULL)`)

	// PatchSessionState shape: sets then deletes over JSONBBase.
	buildPatch := func(sets map[string]string, deletes []string) (string, []any) {
		expr := d.JSONBBase("state_json")
		var args []any
		for _, k := range sortedKeys(sets) {
			expr = d.JSONSet(expr, k, "?")
			args = append(args, sets[k])
		}
		for _, k := range deletes {
			sqlExpr, arg := d.JSONRemove(expr, k)
			expr = sqlExpr
			args = append(args, arg)
		}
		args = append(args, "ok")
		return d.RenumberPlaceholders("UPDATE t_dialect_probe SET state_json = " + expr + " WHERE id = ?"), args
	}

	q, args := buildPatch(
		map[string]string{"run_status": "running", "run_id": "r-1"},
		nil,
	)
	mustExec(q, args...)

	q, args = buildPatch(
		map[string]string{"run_status": "completed"},
		[]string{"run_id"},
	)
	mustExec(q, args...)

	var got string
	if err := db.QueryRowContext(ctx, `SELECT state_json FROM t_dialect_probe WHERE id = 'ok'`).Scan(&got); err != nil {
		t.Fatalf("select: %v", err)
	}
	want := `{"run_status": "completed"}`
	if got != want {
		t.Errorf("state_json = %q, want %q", got, want)
	}

	// Empty-string and NULL rows must be handled by the COALESCE guard.
	for _, id := range []string{"empty", "null"} {
		expr := d.JSONSet(d.JSONBBase("state_json"), "k", "?")
		mustExec(d.RenumberPlaceholders("UPDATE t_dialect_probe SET state_json = "+expr+" WHERE id = ?"), "v", id)
		var s string
		if err := db.QueryRowContext(ctx, `SELECT state_json FROM t_dialect_probe WHERE id = $1`, id).Scan(&s); err != nil {
			t.Fatalf("select %s: %v", id, err)
		}
		if s != `{"k": "v"}` {
			t.Errorf("row %s: state_json = %q, want %q", id, s, `{"k": "v"}`)
		}
	}

	mustExec(`DROP TABLE t_dialect_probe`)
}

// TestDialectIntegration_JSONRemoveMulti executes the JSONRemoveMulti shape
// used by RevertCascadeFactStatements against a real Postgres TEXT column.
func TestDialectIntegration_JSONRemoveMulti(t *testing.T) {
	db := dialectIntegrationDB(t)
	ctx := context.Background()
	d := DialectPostgres

	if _, err := db.ExecContext(ctx, `CREATE TEMP TABLE t_dialect_probe2 (id text PRIMARY KEY, metadata_json text)`); err != nil {
		t.Fatalf("create temp table: %v", err)
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO t_dialect_probe2 (id, metadata_json) VALUES ('f1', '{"keep":"1","a":"x","b":"y"}')`); err != nil {
		t.Fatalf("seed: %v", err)
	}

	expr := d.JSONRemoveMulti(d.JSONBBase("metadata_json"), "a", "b")
	q := d.RenumberPlaceholders("UPDATE t_dialect_probe2 SET metadata_json = " + expr + " WHERE id = ?")
	if _, err := db.ExecContext(ctx, q, "f1"); err != nil {
		t.Fatalf("update: %v\nSQL: %s", err, q)
	}

	var got string
	if err := db.QueryRowContext(ctx, `SELECT metadata_json FROM t_dialect_probe2 WHERE id = 'f1'`).Scan(&got); err != nil {
		t.Fatalf("select: %v", err)
	}
	if want := `{"keep": "1"}`; got != want {
		t.Errorf("metadata_json = %q, want %q", got, want)
	}
	if _, err := db.ExecContext(ctx, `DROP TABLE t_dialect_probe2`); err != nil {
		t.Fatalf("drop: %v", err)
	}
}

// TestDialectIntegration_PlaceholderRenumber locks in the convention that raw
// SQLite-style ? placeholders are rejected by lib/pq and every raw query must
// pass through RenumberPlaceholders. If a future driver swap makes ? valid,
// this test's assumptions must be revisited.
func TestDialectIntegration_PlaceholderRenumber(t *testing.T) {
	db := dialectIntegrationDB(t)
	ctx := context.Background()
	d := DialectPostgres

	if _, err := db.ExecContext(ctx, `SELECT 1 WHERE 1 = ?`, 1); err == nil {
		t.Fatal("expected raw ? placeholder to fail on lib/pq, but it succeeded")
	}

	var one int
	if err := db.QueryRowContext(ctx, d.RenumberPlaceholders(`SELECT 1 WHERE 1 = ?`), 1).Scan(&one); err != nil {
		t.Fatalf("renumbered query failed: %v", err)
	}
	if one != 1 {
		t.Errorf("got %d, want 1", one)
	}
}
