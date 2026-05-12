package provider

import (
	"errors"
	"strings"
)

var (
	ErrAnthropicNativeEndpoint = errors.New("provider: native Anthropic api.anthropic.com is not supported; use an OpenAI-compatible gateway or another driver")
	ErrNilLlmCatalog           = errors.New("provider: llm catalog is nil")
)

func IsLikelyAnthropicNativeAPI(baseURL, _ string) bool {
	b := strings.ToLower(strings.TrimSpace(baseURL))
	if strings.Contains(b, "openrouter") {
		return false
	}
	return strings.Contains(b, "api.anthropic.com")
}
