package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindSystemPSQLPrefersExistingInstall(t *testing.T) {
	bin, psql := findSystemPSQL()
	if psql == "" {
		t.Skip("no system PostgreSQL on this machine")
	}
	if _, err := os.Stat(psql); err != nil {
		t.Fatalf("psql path invalid: %s", psql)
	}
	if filepath.Base(psql) != "psql.exe" {
		t.Fatalf("unexpected psql: %s", psql)
	}
	if bin == "" {
		t.Fatal("empty bin dir")
	}
}

func TestPSQLArgsNeverPrompt(t *testing.T) {
	env := &runtimeEnv{PGUser: "postgres", PGHost: "127.0.0.1", PGPort: "5432"}
	args := psqlArgs(env, "postgres", "-tAc", "SELECT 1")
	if len(args) == 0 || args[0] != "-w" {
		t.Fatalf("psql must start with -w to avoid password hang, got %#v", args)
	}
}

func TestURLQueryEscape(t *testing.T) {
	got := urlQueryEscape("Hangshan@123")
	if got != "Hangshan%40123" {
		t.Fatalf("got %q", got)
	}
}

func TestReportTextContainsLevels(t *testing.T) {
	e := &runtimeEnv{}
	e.add("PostgreSQL", checkOK, "system :5432", false)
	e.add("pgvector", checkWarn, "missing", false)
	e.add("后端", checkFail, "missing exe", true)
	s := e.reportText()
	if !containsAll(s, "[OK]", "[WARN]", "[FAIL]", "PostgreSQL") {
		t.Fatalf("report incomplete: %s", s)
	}
	if !e.hasFatal() || !e.hasWarn() {
		t.Fatal("expected fatal and warn")
	}
}

func containsAll(s string, parts ...string) bool {
	for _, p := range parts {
		if !contains(s, p) {
			return false
		}
	}
	return true
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		(func() bool {
			for i := 0; i+len(sub) <= len(s); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
			return false
		})())
}
