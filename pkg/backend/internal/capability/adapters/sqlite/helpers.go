package sqlite

import (
	"strings"
	"time"
)

func nowISO() string {
	return time.Now().UTC().Format(time.RFC3339)
}

// previewText returns the first `limit` runes of `value` with an ellipsis when truncated.
func previewText(value string, limit int) string {
	value = strings.TrimSpace(value)
	if limit <= 0 || len([]rune(value)) <= limit {
		return value
	}
	runes := []rune(value)
	return string(runes[:limit]) + "..."
}
