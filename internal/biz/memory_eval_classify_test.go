package biz

import "testing"

func TestClassifyEvalFactKind(t *testing.T) {
	t.Parallel()
	cases := []struct {
		stmt string
		want string
	}{
		{"user: I like blue", "preference"},
		{"My favorite color is red", "preference"},
		{"I prefer tea", "preference"},
		{"My name is Alice", "user_identity"},
		{"employee id 8821", "user_identity"},
		{"You must never share the password", "constraint"},
		{"Always use the staging gate", "event"},
		{"I always drink coffee", "event"},
		{"we shipped the patch yesterday", "event"},
	}
	for _, tc := range cases {
		if got := ClassifyEvalFactKind(tc.stmt); got != tc.want {
			t.Errorf("ClassifyEvalFactKind(%q) = %q, want %q", tc.stmt, got, tc.want)
		}
	}
}

func TestShouldSupersedeEvalFact(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		oldKind string
		oldStmt string
		newKind string
		newStmt string
		want    bool
	}{
		{"favorite color update", "preference", "My favorite color is blue", "preference", "My favorite color is red", true},
		{"like without cue coexist", "preference", "I like coffee", "preference", "I like tea", false},
		{"like with now cue", "preference", "I like blue", "preference", "I like red now", true},
		{"live update", "preference", "I live in Boston", "preference", "I live in Seattle", true},
		{"events never supersede", "event", "we shipped the patch", "event", "we shipped another patch", false},
		{"identical skipped", "preference", "I like blue", "preference", "I like blue.", false},
	}
	for _, tc := range cases {
		if got := ShouldSupersedeEvalFact(tc.oldKind, tc.oldStmt, tc.newKind, tc.newStmt); got != tc.want {
			t.Errorf("%s: got %v want %v", tc.name, got, tc.want)
		}
	}
}
