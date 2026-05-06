package provider

import (
	"errors"
	"strings"
)

var (
	// ErrAnthropicNativeEndpoint means the catalog points at Anthropic's native Messages API host;
	// this package only implements OpenAI-compatible /chat/completions today.
	ErrAnthropicNativeEndpoint = errors.New("provider: native Anthropic api.anthropic.com is not supported; use an OpenAI-compatible gateway or another driver")
)

// IsLikelyAnthropicNativeAPI matches internal/agent.IsLikelyAnthropicNativeAPI (kept here to avoid import cycles).
func IsLikelyAnthropicNativeAPI(baseURL, _ string) bool {
	b := strings.ToLower(strings.TrimSpace(baseURL))
	if strings.Contains(b, "openrouter") {
		return false
	}
	return strings.Contains(b, "api.anthropic.com")
}
