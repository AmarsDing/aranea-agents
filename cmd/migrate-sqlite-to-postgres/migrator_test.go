package main

import "testing"

func TestQuoteIdent(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"users", `"users"`},
		{"agent_runtime_setting", `"agent_runtime_setting"`},
		// Internal double quotes are escaped by doubling (SQL standard).
		{`a"b`, `"a""b"`},
		// Two double quotes -> each doubled -> four, plus wrapping quotes -> six.
		{`""`, `""""""`},
		// Empty string is a valid (if unusual) identifier.
		{"", `""`},
	}
	for _, c := range cases {
		got := quoteIdent(c.in)
		if got != c.want {
			t.Errorf("quoteIdent(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestQuoteIdentifiers(t *testing.T) {
	got := quoteIdentifiers([]string{"id", "name", `with"quote`})
	want := []string{`"id"`, `"name"`, `"with""quote"`}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("quoteIdentifiers[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestQuoteIdentifiersEmpty(t *testing.T) {
	got := quoteIdentifiers(nil)
	if len(got) != 0 {
		t.Fatalf("expected empty slice, got %v", got)
	}
}
