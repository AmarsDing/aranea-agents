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

// ResolveWindow picks context window tokens for the active model call.
// Priority: provider model config → session default → agent default → fallback.
func ResolveWindow(in ResolveInput) int {
	if w := contextWindowFromConfigJSON(in.ProviderModelConfigJSON); w > 0 {
		return w
	}
	if in.SessionDefaultWindow > 0 {
		return in.SessionDefaultWindow
	}
	if in.AgentWindow > 0 {
		return in.AgentWindow
	}
	return DefaultWindowTokens
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
