package streamgen

import "testing"

func TestLabelEmptyWhenNeverBumped(t *testing.T) {
	if got := Label("inv-never"); got != "" {
		t.Fatalf("Label(never bumped) = %q, want empty", got)
	}
	if got := Label(""); got != "" {
		t.Fatalf("Label(empty) = %q, want empty", got)
	}
}

func TestBumpSequence(t *testing.T) {
	id := "inv-bump-seq"
	if got := Bump(id); got != "reconnect-1" {
		t.Fatalf("first Bump = %q, want reconnect-1", got)
	}
	if got := Label(id); got != "reconnect-1" {
		t.Fatalf("Label after first Bump = %q, want reconnect-1", got)
	}
	if got := Bump(id); got != "reconnect-2" {
		t.Fatalf("second Bump = %q, want reconnect-2", got)
	}
	if got := Label(id); got != "reconnect-2" {
		t.Fatalf("Label after second Bump = %q, want reconnect-2", got)
	}
	// 其他 invocation 不受影响。
	if got := Label("inv-bump-other"); got != "" {
		t.Fatalf("Label(other) = %q, want empty", got)
	}
}

func TestBumpEmptyIDNoop(t *testing.T) {
	if got := Bump(""); got != "" {
		t.Fatalf("Bump(empty) = %q, want empty", got)
	}
	if len(entries) != 0 && entries[""] != nil {
		t.Fatal("Bump(empty) must not create an entry")
	}
}
