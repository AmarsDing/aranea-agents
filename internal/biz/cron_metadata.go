package biz

import (
	"encoding/json"
	"strings"
)

func resetCronFailureMetadata(raw string) (string, error) {
	meta := map[string]any{}
	if strings.TrimSpace(raw) != "" {
		if err := json.Unmarshal([]byte(raw), &meta); err != nil {
			return "", err
		}
	}
	meta["failure_count"] = 0
	meta["last_error"] = ""
	meta["recent_failures"] = []any{}
	out, err := json.Marshal(meta)
	if err != nil {
		return "", err
	}
	return string(out), nil
}
