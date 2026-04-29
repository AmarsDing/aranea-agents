package middleware

import (
	"strings"
	"testing"
)

func TestSanitizeArgsRedactsSensitiveKeys(t *testing.T) {
	out := sanitizeArgs(map[string]any{
		"Authorization": "Bearer abc",
		"api_key":       "secret-key",
		"password":      "hunter2",
		"path":          "docs/file.md",
	})

	for _, key := range []string{"Authorization", "api_key", "password"} {
		if out[key] != "***" {
			t.Fatalf("expected %s to be redacted, got %#v", key, out[key])
		}
	}
	if out["path"] != "docs/file.md" {
		t.Fatalf("expected non-sensitive value to pass through, got %#v", out["path"])
	}
}

func TestSanitizeArgsTruncatesLargeStrings(t *testing.T) {
	out := sanitizeArgs(map[string]any{
		"content": strings.Repeat("a", maxEventStringRunes+10),
	})

	text, ok := out["content"].(string)
	if !ok {
		t.Fatalf("expected content to remain a string, got %T", out["content"])
	}
	if len([]rune(text)) != maxEventStringRunes+3 {
		t.Fatalf("expected truncated string with ellipsis, got length %d", len([]rune(text)))
	}
	if !strings.HasSuffix(text, "...") {
		t.Fatalf("expected truncated string to end with ellipsis")
	}
}
