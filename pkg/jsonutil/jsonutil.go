package jsonutil

import (
	"encoding/json"
	"strconv"
	"strings"
)

func IfaceStr(m map[string]any, k string) string {
	v, ok := m[k]
	if !ok || v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	case float64:
		return strconv.FormatInt(int64(t), 10)
	case bool:
		return strconv.FormatBool(t)
	default:
		return ""
	}
}

func IfaceBool(m map[string]any, k string) bool {
	v, ok := m[k]
	if !ok || v == nil {
		return false
	}
	switch t := v.(type) {
	case bool:
		return t
	case float64:
		return t != 0
	default:
		return false
	}
}

func IfaceF64(m map[string]any, k string) float64 {
	v, ok := m[k]
	if !ok || v == nil {
		return 0
	}
	switch t := v.(type) {
	case float64:
		return t
	case float32:
		return float64(t)
	case int:
		return float64(t)
	case int64:
		return float64(t)
	default:
		return 0
	}
}

func IfaceI32(m map[string]any, k string) int32 {
	v, ok := m[k]
	if !ok || v == nil {
		return 0
	}
	switch t := v.(type) {
	case float64:
		return int32(t)
	case int:
		return int32(t)
	default:
		return 0
	}
}

func IfaceInt(m map[string]any, k string) int {
	v, ok := m[k]
	if !ok || v == nil {
		return 0
	}
	switch t := v.(type) {
	case float64:
		return int(t)
	case int:
		return t
	case int64:
		return int(t)
	default:
		return 0
	}
}

func ParseMap(raw []byte) (map[string]any, error) {
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	if m == nil {
		return map[string]any{}, nil
	}
	return m, nil
}

// MapStringFloat extracts float64 values from a map[string]any,
// converting int values to float64.
func MapStringFloat(in map[string]any) map[string]float64 {
	out := make(map[string]float64)
	for k, v := range in {
		switch t := v.(type) {
		case float64:
			out[k] = t
		case int:
			out[k] = float64(t)
		}
	}
	return out
}
