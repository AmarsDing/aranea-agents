package sessionmemory

import (
	"strings"
	"time"
)

// relationValidAt reports whether a relation is active at queryTime.
// Empty validFrom defaults to createdAt; empty validTo means open-ended.
func relationValidAt(validFrom, validTo, createdAt, queryTime string) bool {
	q, ok := parseMemoryTime(queryTime)
	if !ok {
		return true
	}
	fromText := strings.TrimSpace(validFrom)
	if fromText == "" {
		fromText = strings.TrimSpace(createdAt)
	}
	if fromText != "" {
		if from, ok := parseMemoryTime(fromText); ok && q.Before(from) {
			return false
		}
	}
	toText := strings.TrimSpace(validTo)
	if toText != "" {
		if to, ok := parseMemoryTime(toText); ok && q.After(to) {
			return false
		}
	}
	return true
}

func parseMemoryTime(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t, true
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, true
	}
	return time.Time{}, false
}

func defaultQueryTimeRFC3339(queryAt string) string {
	if strings.TrimSpace(queryAt) != "" {
		return strings.TrimSpace(queryAt)
	}
	return time.Now().UTC().Format(time.RFC3339Nano)
}
