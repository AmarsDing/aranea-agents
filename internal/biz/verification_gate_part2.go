package biz

import (
	"encoding/json"
	"strings"
	"unicode/utf8"
)

// parseGateResult parses the LLM response into a GateResult.
func parseGateResult(resp string) (GateResult, error) {

	cleaned := resp
	if idx := strings.Index(resp, "{"); idx >= 0 {
		cleaned = resp[idx:]
	}
	if idx := strings.LastIndex(cleaned, "}"); idx >= 0 {
		cleaned = cleaned[:idx+1]
	}

	var result GateResult
	if err := json.Unmarshal([]byte(cleaned), &result); err != nil {
		return GateResult{}, err
	}
	return result, nil
}

// ParseVerificationGates parses a JSON string into a slice of VerificationGate.
func ParseVerificationGates(jsonStr string) ([]VerificationGate, error) {
	if jsonStr == "" {
		return nil, nil
	}
	var gates []VerificationGate
	if err := json.Unmarshal([]byte(jsonStr), &gates); err != nil {
		return nil, err
	}
	return gates, nil
}

// VerificationGatesToJSON serializes a slice of VerificationGate to JSON.
func VerificationGatesToJSON(gates []VerificationGate) string {
	if len(gates) == 0 {
		return "[]"
	}
	b, err := json.Marshal(gates)
	if err != nil {
		return "[]"
	}
	return string(b)
}

// truncateForPrompt truncates content for inclusion in LLM prompts.
func truncateForPrompt(s string, maxRunes int) string {
	if utf8.RuneCountInString(s) <= maxRunes {
		return s
	}
	runes := []rune(s)
	return string(runes[:maxRunes]) + "..."
}
