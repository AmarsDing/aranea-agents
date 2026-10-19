package data

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"
)

func truncateRunes(s string, n int) string {
	if n <= 0 || s == "" {
		return s
	}
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n])
}

// normalizeEvalTimestamp accepts RFC3339(Nano) strings or epoch seconds/millis
// and returns RFC3339Nano; unparseable input yields "" (repo defaults to now).
func normalizeEvalTimestamp(ts string) string {
	ts = strings.TrimSpace(ts)
	if ts == "" {
		return ""
	}
	if t, err := time.Parse(time.RFC3339Nano, ts); err == nil {
		return t.UTC().Format(time.RFC3339Nano)
	}
	if n, err := strconv.ParseInt(ts, 10, 64); err == nil {
		if n > 1e12 {
			return time.UnixMilli(n).UTC().Format(time.RFC3339Nano)
		}
		return time.Unix(n, 0).UTC().Format(time.RFC3339Nano)
	}
	return ""
}

func evalMetadataJSON(sessionID string, seq int) string {
	b, err := json.Marshal(map[string]any{"eval_session": sessionID, "eval_seq": seq})
	if err != nil {
		return "{}"
	}
	return string(b)
}
