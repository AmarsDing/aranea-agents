package data

import "testing"

func TestFtsMatchQuery(t *testing.T) {
	got := ftsMatchQuery("hello world")
	if got != `"hello" "world"` {
		t.Fatalf("got %q", got)
	}
}

func TestEscapeLike(t *testing.T) {
	got := escapeLike(`50%_off`)
	if got != `50\%\_off` {
		t.Fatalf("got %q", got)
	}
}
