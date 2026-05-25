package llmcontext

import (
	"strings"
	"unicode/utf8"
)

// RoughTokenEstimate approximates token count (~4 runes per token) when providers omit usage.
func RoughTokenEstimate(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	n := utf8.RuneCountInString(s)
	if n < 1 {
		return 0
	}
	est := n / 4
	if est < 1 {
		return 1
	}
	return est
}
