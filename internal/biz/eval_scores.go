package biz

import "encoding/json"

// EvalScores holds arbitrary metric key → score mappings (extended framework metrics).
type EvalScores map[string]float32

// ParseEvalScores unmarshals scores_json; invalid input yields empty map.
func ParseEvalScores(raw string) EvalScores {
	if raw == "" || raw == "{}" {
		return EvalScores{}
	}
	var m EvalScores
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return EvalScores{}
	}
	return m
}

// MarshalEvalScores serializes scores to JSON object string.
func MarshalEvalScores(m EvalScores) string {
	if len(m) == 0 {
		return "{}"
	}
	b, err := json.Marshal(m)
	if err != nil {
		return "{}"
	}
	return string(b)
}
