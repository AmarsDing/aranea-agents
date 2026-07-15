package main

import "testing"

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
