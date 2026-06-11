package importer

import "testing"

func TestDirectorySlugMismatch(t *testing.T) {
	if issue := DirectorySlugMismatch("daily-report", "daily-report"); issue != nil {
		t.Fatalf("expected match, got %v", issue)
	}
	if issue := DirectorySlugMismatch("foo", "bar-baz"); issue == nil {
		t.Fatal("expected mismatch")
	}
}

func TestSlugifyOrRandom(t *testing.T) {
	if got := SlugifyOrRandom("Daily Report"); got != "daily-report" {
		t.Fatalf("got %q", got)
	}
}
