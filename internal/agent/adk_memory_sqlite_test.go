package agent

import "testing"

func TestNormalizeMemoryEntityKey(t *testing.T) {
	if got := normalizeMemoryEntityKey("hello-world"); got != "hello-world" {
		t.Fatalf("got %q", got)
	}
	if got := normalizeMemoryEntityKey("Evt: 42!"); got != "evt___42" {
		t.Fatalf("got %q", got)
	}
	if got := normalizeMemoryEntityKey(""); got != "event" {
		t.Fatalf("got %q", got)
	}
}
