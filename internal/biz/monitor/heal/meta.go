package heal

import (
	"encoding/json"
	"fmt"
	"strings"
)

// metaStr extracts a trimmed string value from a metadata map. Shared by the
// rca/heal domain files (the trace subpackage keeps its own private copy).
func metaStr(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(v))
}

// metaStrFromMap extracts a string value from a map[string]any, similar to metaStr
// but works with HealRecord.Metadata directly (strict string type assertion, no
// fmt.Sprint coercion).
func metaStrFromMap(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	v, ok := m[key]
	if !ok {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return s
}

func parseMetadataJSON(raw string) map[string]any {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var m map[string]any
	if json.Unmarshal([]byte(raw), &m) != nil {
		return nil
	}
	return m
}

func nonEmpty(ss ...string) []string {
	var out []string
	for _, s := range ss {
		if strings.TrimSpace(s) != "" {
			out = append(out, s)
		}
	}
	return out
}
