package llmcontext

import (
	"encoding/json"
	"strings"
)

const DefaultWindowTokens = 128000

type ResolveInput struct {
	ProviderModelConfigJSON string
	SessionDefaultWindow    int
	AgentWindow             int
}

// ResolveWindow picks the effective context window tokens for the active model
// call. All inputs are treated as candidate ceilings: the provider catalog value
// states the vendor-claimed maximum, while session/agent values are local
// operational caps. The smallest positive value wins, so a local cap always
// constrains an inflated catalog value (a 1M-token catalog entry would otherwise
// push compression trigger thresholds to ~700K and sessions would never
// compact). Falls back to DefaultWindowTokens when no input is set.
func ResolveWindow(in ResolveInput) int {
	win := 0
	consider := func(v int) {
		if v > 0 && (win <= 0 || v < win) {
			win = v
		}
	}
	consider(contextWindowFromConfigJSON(in.ProviderModelConfigJSON))
	consider(in.SessionDefaultWindow)
	consider(in.AgentWindow)
	if win <= 0 {
		return DefaultWindowTokens
	}
	return win
}

func contextWindowFromConfigJSON(raw string) int {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "{}" {
		return 0
	}
	var cfg struct {
		ContextWindowK int `json:"context_window_k"`
	}
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return 0
	}
	if cfg.ContextWindowK <= 0 {
		return 0
	}
	return cfg.ContextWindowK * 1000
}
