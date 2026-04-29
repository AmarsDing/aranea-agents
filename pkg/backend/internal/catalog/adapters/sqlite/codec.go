package sqlite

import (
	mem "arenea/backend/internal/memory/domain"

	"encoding/json"
	"strings"
	"time"
)

// rowScanner 抽象 *sql.Row 与 *sql.Rows，与 repository 层 scan 辅助一致。
type rowScanner interface {
	Scan(dest ...any) error
}

func nowISO() string {
	return time.Now().UTC().Format(time.RFC3339)
}

func normalizeJSONList(value string) string {
	if strings.TrimSpace(value) == "" {
		return "[]"
	}
	if json.Valid([]byte(value)) {
		return value
	}
	encoded, err := json.Marshal([]string{value})
	if err != nil {
		return "[]"
	}
	return string(encoded)
}

func decodeJSONFloatMap(raw string) map[string]float64 {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" {
		return nil
	}
	out := map[string]float64{}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	return out
}

func encodeJSONFloatMap(in map[string]float64) string {
	if len(in) == 0 {
		return "{}"
	}
	b, err := json.Marshal(in)
	if err != nil {
		return "{}"
	}
	return string(b)
}

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

func encodeJSONStringSlice(in []string) string {
	if len(in) == 0 {
		return "[]"
	}
	b, err := json.Marshal(in)
	if err != nil {
		return "[]"
	}
	return string(b)
}

func encodeJSONObject(in map[string]any) string {
	if len(in) == 0 {
		return "{}"
	}
	b, err := json.Marshal(in)
	if err != nil {
		return "{}"
	}
	return string(b)
}

func decodeEvidenceList(raw string) []mem.EvidenceRef {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" {
		return nil
	}
	var out []mem.EvidenceRef
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	return out
}

func encodeEvidenceList(in []mem.EvidenceRef) string {
	if len(in) == 0 {
		return "[]"
	}
	b, err := json.Marshal(in)
	if err != nil {
		return "[]"
	}
	return string(b)
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func sanitizePromptFileID(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			return r
		}
		return '_'
	}, value)
	return strings.Trim(value, "_")
}
