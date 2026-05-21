package preview

import (
	"strings"
	"testing"
)

func TestRedactAndTruncate_masksSecrets(t *testing.T) {
	in := `{"api_key":"sk-live-secret","email":"a@b.com"}`
	out := RedactAndTruncate(in, 2000)
	if strings.Contains(out, "sk-live-secret") || strings.Contains(out, "a@b.com") {
		t.Fatalf("expected redaction, got %q", out)
	}
}
