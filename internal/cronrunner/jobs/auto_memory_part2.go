package jobs

import (
	"encoding/json"
)

func jsonNumber(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case json.Number:
		f, _ := n.Float64()
		return f
	default:
		return 0
	}
}

// errString safely converts an error to its string representation.
// Returns empty string for nil errors.
func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
