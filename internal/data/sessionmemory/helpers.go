package sessionmemory

import (
	"encoding/json"
	"strings"
)

func decodeJSONStringSlice(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" {
		return nil
	}
	var out []string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	return out
}

func decodeJSONObject(raw string) map[string]any {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" {
		return nil
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	return out
}

func decodeJSONFloatMap(raw string) map[string]float64 {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" || raw == "{}" {
		return map[string]float64{}
	}
	var out map[string]float64
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return map[string]float64{}
	}
	return out
}
