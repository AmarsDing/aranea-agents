package llmcontext

import (
	"context"
	"encoding/json"
	"strings"
)

type windowOverrideKey struct{}

// ContextWithWindow overrides the product chat-context budget on ctx.
// Production chat/compression paths must not set this; it exists so unit
// tests can exercise truncation with a miniature window.
func ContextWithWindow(ctx context.Context, tokens int) context.Context {
	if ctx == nil || tokens <= 0 {
		return ctx
	}
	return context.WithValue(ctx, windowOverrideKey{}, tokens)
}

// WindowFromContext returns the ctx override when set, otherwise the 256K
// product budget.
func WindowFromContext(ctx context.Context) int {
	if ctx != nil {
		if v, ok := ctx.Value(windowOverrideKey{}).(int); ok && v > 0 {
			return v
		}
	}
	return DefaultWindowTokens
}

// DefaultWindowTokens is the product chat-context budget and compression
// standard for every model (256K). Vendor-claimed provider windows are
// informational only and must not drive chat ratio, UI, or compression.
const DefaultWindowTokens = 256000

// MaxWindowTokens is the hard ceiling for chat context support (256K).
const MaxWindowTokens = 256000

type ResolveInput struct {
	ProviderModelConfigJSON string
	SessionDefaultWindow    int
	AgentWindow             int
}

// ResolveWindow returns the product chat-context budget. Provider catalog
// context_window_k, session defaults, and agent.context_window are not used:
// those values describe vendor or local model metadata, not Aranea's chat
// context support. The budget is a fixed 256K for every model.
func ResolveWindow(_ ResolveInput) int {
	return DefaultWindowTokens
}

// ClampWindow caps a token count to the product chat-context ceiling.
// Non-positive values fall back to DefaultWindowTokens.
func ClampWindow(tokens int) int {
	if tokens <= 0 {
		return DefaultWindowTokens
	}
	if tokens > MaxWindowTokens {
		return MaxWindowTokens
	}
	return tokens
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
