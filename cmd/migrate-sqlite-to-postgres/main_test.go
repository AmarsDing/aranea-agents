package main

import "testing"

func TestParseSkipTables(t *testing.T) {
	cases := []struct {
		in   string
		want map[string]bool
	}{
		{"", map[string]bool{}},
		{"users", map[string]bool{"users": true}},
		{"users,agents,sessions", map[string]bool{"users": true, "agents": true, "sessions": true}},
		// Whitespace is trimmed.
		{" users , agents , sessions ", map[string]bool{"users": true, "agents": true, "sessions": true}},
		// Empty entries between commas are skipped.
		{"users,,agents,", map[string]bool{"users": true, "agents": true}},
	}
	for _, c := range cases {
		got := parseSkipTables(c.in)
		if len(got) != len(c.want) {
			t.Errorf("parseSkipTables(%q) = %v (len %d), want %v (len %d)", c.in, got, len(got), c.want, len(c.want))
			continue
		}
		for k := range c.want {
			if !got[k] {
				t.Errorf("parseSkipTables(%q): missing key %q", c.in, k)
			}
		}
	}
}

// TestParseSkipTablesReturnsNewMap ensures the function always returns a non-nil
// map so callers can safely write to it.
func TestParseSkipTablesReturnsNewMap(t *testing.T) {
	got := parseSkipTables("")
	if got == nil {
		t.Fatal("expected non-nil map for empty input")
	}
	got["x"] = true
	if !got["x"] {
		t.Fatal("map should be writable")
	}
}
