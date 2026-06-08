package biz

import (
	"encoding/json"
	"testing"
)

func TestFactFingerprint(t *testing.T) {
	tests := []struct {
		name      string
		statement string
		scopeType string
		scopeID   string
	}{
		{"basic", "User prefers dark mode", "agent", "ag1"},
		{"case insensitive", "USER PREFERS DARK MODE", "agent", "ag1"},
		{"whitespace trimmed", "  User prefers dark mode  ", "  agent  ", "  ag1  "},
		{"empty scope", "Hello", "", ""},
	}

	// Same logical content should produce same fingerprint
	fp1 := FactFingerprint("User prefers dark mode", "agent", "ag1")
	fp2 := FactFingerprint("user prefers dark mode", "agent", "ag1")
	if fp1 != fp2 {
		t.Errorf("case-insensitive mismatch: %q vs %q", fp1, fp2)
	}

	fp3 := FactFingerprint("  User prefers dark mode  ", "  agent  ", "  ag1  ")
	if fp1 != fp3 {
		t.Errorf("whitespace-trimmed mismatch: %q vs %q", fp1, fp3)
	}

	// Different content should produce different fingerprint
	fp4 := FactFingerprint("User prefers light mode", "agent", "ag1")
	if fp1 == fp4 {
		t.Error("different statements should not have same fingerprint")
	}

	// Different scope should produce different fingerprint
	fp5 := FactFingerprint("User prefers dark mode", "user", "u1")
	if fp1 == fp5 {
		t.Error("different scopes should not have same fingerprint")
	}

	// Fingerprint should be 64 hex chars (SHA-256)
	for _, tt := range tests {
		fp := FactFingerprint(tt.statement, tt.scopeType, tt.scopeID)
		if len(fp) != 64 {
			t.Errorf("%s: fingerprint length = %d, want 64", tt.name, len(fp))
		}
	}

	// Empty statement should still produce a valid fingerprint
	fpEmpty := FactFingerprint("", "agent", "ag1")
	if len(fpEmpty) != 64 {
		t.Errorf("empty statement fingerprint length = %d, want 64", len(fpEmpty))
	}
}

func TestNormalizeForDedup(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Hello World", "hello world"},
		{"  Hello  World  ", "hello world"},
		{"Hello\tWorld\nTest", "hello world test"},
		{"", ""},
		{"  ", ""},
		{"UPPERCASE", "uppercase"},
	}
	for _, tt := range tests {
		got := NormalizeForDedup(tt.input)
		if got != tt.want {
			t.Errorf("NormalizeForDedup(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestDedupL3WithL1(t *testing.T) {
	makeRow := func(statement string) []byte {
		b, _ := json.Marshal(map[string]any{"statement": statement})
		return b
	}

	t.Run("no L1 fields returns all L3 rows", func(t *testing.T) {
		rows := [][]byte{makeRow("fact 1"), makeRow("fact 2")}
		got := DedupL3WithL1(rows, nil)
		if len(got) != 2 {
			t.Errorf("got %d rows, want 2", len(got))
		}
	})

	t.Run("no L3 rows returns empty", func(t *testing.T) {
		got := DedupL3WithL1(nil, []string{"some value"})
		if len(got) != 0 {
			t.Errorf("got %d rows, want 0", len(got))
		}
	})

	t.Run("filters exact match", func(t *testing.T) {
		rows := [][]byte{makeRow("User prefers dark mode"), makeRow("User likes tea")}
		l1 := []string{"User prefers dark mode"}
		got := DedupL3WithL1(rows, l1)
		if len(got) != 1 {
			t.Fatalf("got %d rows, want 1", len(got))
		}
		var row map[string]any
		json.Unmarshal(got[0], &row)
		if row["statement"] != "User likes tea" {
			t.Errorf("kept wrong row: %v", row["statement"])
		}
	})

	t.Run("filters case-insensitive match", func(t *testing.T) {
		rows := [][]byte{makeRow("USER PREFERS DARK MODE")}
		l1 := []string{"user prefers dark mode"}
		got := DedupL3WithL1(rows, l1)
		if len(got) != 0 {
			t.Errorf("got %d rows, want 0 (case-insensitive match)", len(got))
		}
	})

	t.Run("filters whitespace-normalized match", func(t *testing.T) {
		rows := [][]byte{makeRow("User  prefers   dark mode")}
		l1 := []string{"User prefers dark mode"}
		got := DedupL3WithL1(rows, l1)
		if len(got) != 0 {
			t.Errorf("got %d rows, want 0 (whitespace-normalized match)", len(got))
		}
	})

	t.Run("keeps non-matching rows", func(t *testing.T) {
		rows := [][]byte{makeRow("User prefers dark mode"), makeRow("User likes tea"), makeRow("System uses Go")}
		l1 := []string{"User prefers dark mode"}
		got := DedupL3WithL1(rows, l1)
		if len(got) != 2 {
			t.Errorf("got %d rows, want 2", len(got))
		}
	})

	t.Run("invalid JSON row is kept", func(t *testing.T) {
		rows := [][]byte{[]byte("not json"), makeRow("User likes tea")}
		l1 := []string{"User likes tea"}
		got := DedupL3WithL1(rows, l1)
		if len(got) != 1 {
			t.Errorf("got %d rows, want 1 (invalid JSON kept)", len(got))
		}
	})

	t.Run("empty L1 values are ignored", func(t *testing.T) {
		rows := [][]byte{makeRow("fact 1")}
		l1 := []string{"", "  "}
		got := DedupL3WithL1(rows, l1)
		if len(got) != 1 {
			t.Errorf("got %d rows, want 1 (empty L1 values ignored)", len(got))
		}
	})
}
