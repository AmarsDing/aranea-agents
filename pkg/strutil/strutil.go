package strutil

import (
	"strings"
	"unicode/utf8"
)

func FirstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// TruncateRunes shortens s to at most maxRunes Unicode code points (safe for UTF-8 / protobuf).
func TruncateRunes(s string, maxRunes int) string {
	if maxRunes <= 0 || s == "" {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	return string(runes[:maxRunes])
}

// ValidUTF8 returns s if valid UTF-8; otherwise strips/replaces invalid sequences for proto string fields.
func ValidUTF8(s string) string {
	if utf8.ValidString(s) {
		return s
	}
	return strings.ToValidUTF8(s, "")
}

// ProtoPreview prepares user-generated text for protobuf string fields.
func ProtoPreview(s string, maxRunes int) string {
	return ValidUTF8(TruncateRunes(s, maxRunes))
}

func SliceToSet(ss []string) map[string]bool {
	m := make(map[string]bool, len(ss))
	for _, s := range ss {
		s = strings.TrimSpace(strings.ToLower(s))
		if s != "" {
			m[s] = true
		}
	}
	return m
}

func TruncateBytes(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n]
}
