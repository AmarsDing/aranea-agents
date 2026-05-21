package data

import (
	"strings"
	"testing"
)

func TestSplitSQLStatements_triggerBlocks(t *testing.T) {
	stmts := splitSQLStatements(messageFTSDDL)
	if len(stmts) != 5 {
		t.Fatalf("got %d statements, want 5: %v", len(stmts), stmts)
	}
	wantPrefixes := []string{
		"CREATE VIRTUAL TABLE",
		"INSERT INTO messages_fts",
		"CREATE TRIGGER IF NOT EXISTS messages_fts_ai",
		"CREATE TRIGGER IF NOT EXISTS messages_fts_ad",
		"CREATE TRIGGER IF NOT EXISTS messages_fts_au",
	}
	for i, prefix := range wantPrefixes {
		if !strings.HasPrefix(stmts[i], prefix) {
			t.Fatalf("stmt[%d]: got %q", i, stmts[i])
		}
	}
	if !strings.HasSuffix(strings.TrimSpace(stmts[2]), "END") {
		t.Fatalf("trigger ai missing END: %q", stmts[2])
	}
}
