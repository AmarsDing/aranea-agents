package biz

import "testing"

func TestRedactToolPreview_masksSecrets(t *testing.T) {
	in := `{"api_key":"sk-live-secret","email":"a@b.com"}`
	out := RedactToolPreview(in, 2000)
	if stringsContains(out, "sk-live-secret") || stringsContains(out, "a@b.com") {
		t.Fatalf("expected redaction, got %q", out)
	}
}

func stringsContains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && indexOfSubstring(s, sub) >= 0)
}

func indexOfSubstring(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
