package webresearch

import "strings"

// truncateUTF8 truncates s to at most max bytes on a UTF-8 rune boundary.
func truncateUTF8(s string, max int) string {
	s = strings.TrimSpace(s)
	if max <= 0 || len(s) <= max {
		return s
	}
	var b strings.Builder
	current := 0
	for _, r := range s {
		rLen := len(string(r))
		if current+rLen > max {
			break
		}
		b.WriteRune(r)
		current += rLen
	}
	if b.Len() == 0 {
		return ""
	}
	return b.String() + "\n...[truncated]"
}
