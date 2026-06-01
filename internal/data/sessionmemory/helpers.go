package sessionmemory

import (
	"encoding/json"
	"strings"

	"aranea-agents/pkg/loggateway"
)

func decodeJSONStringSlice(raw string, lg loggateway.Logger) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" {
		return nil
	}
	var out []string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		lg.Warn("session memory json unmarshal failed", loggateway.StepID("data.sessionmemory"), loggateway.Err(err))
		return nil
	}
	return out
}

func decodeJSONObject(raw string, lg loggateway.Logger) map[string]any {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" {
		return nil
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		lg.Warn("session memory json unmarshal failed", loggateway.StepID("data.sessionmemory"), loggateway.Err(err))
		return nil
	}
	return out
}

func decodeJSONFloatMap(raw string, lg loggateway.Logger) map[string]float64 {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" || raw == "{}" {
		return map[string]float64{}
	}
	var out map[string]float64
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		lg.Warn("session memory json unmarshal failed", loggateway.StepID("data.sessionmemory"), loggateway.Err(err))
		return map[string]float64{}
	}
	return out
}
