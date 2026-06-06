package monitor

import (
	"encoding/json"
	"strings"

	"aranea-agents/pkg/loggateway"
)

// SanitizeJSONString parses raw JSON, redacts sensitive fields, and returns the sanitized JSON string.
func SanitizeJSONString(raw string, lg loggateway.Logger) string {
	parsed := ParseJSONMap(raw, lg)
	if len(parsed) == 0 {
		return raw
	}
	sanitized := SanitizeJSONValue(parsed)
	out, err := json.Marshal(sanitized)
	if err != nil {
		return raw
	}
	return string(out)
}

// ParseJSONMap parses a JSON string into a map, sanitizing sensitive fields.
func ParseJSONMap(raw string, lg loggateway.Logger) map[string]any {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return map[string]any{}
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		lg.Warn("json map unmarshal failed", loggateway.StepID("monitor.parse_json_map"), loggateway.Err(err))
		return map[string]any{}
	}
	return SanitizeJSONValue(parsed).(map[string]any)
}

// SanitizeJSONValue recursively redacts values whose keys match sensitive patterns.
func SanitizeJSONValue(value any) any {
	switch v := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		for key, child := range v {
			if IsSensitiveKey(key) {
				out[key] = "******"
				continue
			}
			out[key] = SanitizeJSONValue(child)
		}
		return out
	case []any:
		out := make([]any, 0, len(v))
		for _, child := range v {
			out = append(out, SanitizeJSONValue(child))
		}
		return out
	default:
		return value
	}
}

// IsSensitiveKey reports whether the key matches a known sensitive field pattern.
func IsSensitiveKey(key string) bool {
	key = strings.ToLower(strings.TrimSpace(key))
	for _, token := range []string{"api_key", "apikey", "token", "secret", "password", "authorization", "cookie"} {
		if strings.Contains(key, token) {
			return true
		}
	}
	return false
}
