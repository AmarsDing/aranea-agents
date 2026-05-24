package preview

import "strings"

// TruncateRunes returns at most maxRunes runes from text (no ellipsis).
func TruncateRunes(text string, maxRunes int) string {
	text = strings.TrimSpace(text)
	if text == "" || maxRunes <= 0 {
		return ""
	}
	runes := []rune(text)
	if len(runes) <= maxRunes {
		return text
	}
	return string(runes[:maxRunes])
}
