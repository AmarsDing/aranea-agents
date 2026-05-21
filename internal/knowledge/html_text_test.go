package knowledge

import (
	"strings"
	"testing"
)

func TestStripHTML(t *testing.T) {
	t.Parallel()
	raw := `<html><head><style>.x{}</style><script>alert(1)</script></head><body><h1>Title</h1><p>Hello <b>world</b></p></body></html>`
	got := stripHTML(raw)
	if got == "" {
		t.Fatal("expected text")
	}
	for _, want := range []string{"Title", "Hello", "world"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in %q", want, got)
		}
	}
	for _, bad := range []string{"alert", ".x"} {
		if strings.Contains(got, bad) {
			t.Fatalf("script/style leaked %q in %q", bad, got)
		}
	}
}
