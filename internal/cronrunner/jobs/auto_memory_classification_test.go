package jobs

import "testing"

func TestMemoryFactKindForSubjectType(t *testing.T) {
	cases := map[string]string{
		"person":     "profile",
		"preference": "preference",
		"constraint": "constraint",
		"event":      "event",
		"concept":    "knowledge",
		"other":      "fact",
		"":           "fact",
		"unknown":    "fact",
	}
	for in, want := range cases {
		if got := memoryFactKindForSubjectType(in); got != want {
			t.Fatalf("memoryFactKindForSubjectType(%q)=%q want %q", in, got, want)
		}
	}
}

func TestResolveFactScope(t *testing.T) {
	st, id := resolveFactScope("user", "u1", "agent-1")
	if st != "user" || id != "u1" {
		t.Fatalf("user scope: got %s/%s", st, id)
	}
	st, id = resolveFactScope("agent", "u1", "agent-1")
	if st != "agent" || id != "agent-1" {
		t.Fatalf("agent scope: got %s/%s", st, id)
	}
	// user scope without userID falls back to agent scope
	st, id = resolveFactScope("user", "", "agent-1")
	if st != "agent" || id != "agent-1" {
		t.Fatalf("user scope without userID should fall back to agent: got %s/%s", st, id)
	}
	// empty scope defaults to agent
	st, id = resolveFactScope("", "u1", "agent-1")
	if st != "agent" || id != "agent-1" {
		t.Fatalf("empty scope should default to agent: got %s/%s", st, id)
	}
}
