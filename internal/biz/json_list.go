package biz

import (
	"encoding/json"
	"strings"
)

// JSONStringList parses a JSON string array from agent settings or API payloads.
func JSONStringList(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "[]" || raw == "{}" {
		return nil
	}
	var list []string
	if err := json.Unmarshal([]byte(raw), &list); err != nil {
		return nil
	}
	return list
}
