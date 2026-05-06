package provider

import (
	"context"
	"errors"
	"strings"
	"sync"

	"aranea-agents/internal/provider/deepseek"
	"aranea-agents/internal/provider/gemini"
	"aranea-agents/internal/provider/openai"
)

// LLMFactory builds a backend from transport + biz-derived connection fields.
type LLMFactory func(rt *RoundTrip, cc CatalogConfig) (LLM, error)

// Registry binds provider_type catalog labels to LLM constructors (same idea as injecting model.LLM in ADK Runner config).
type Registry struct {
	mu       sync.RWMutex
	byType   map[string]LLMFactory
	fallback LLMFactory
}

// NewRegistry wires OpenAI-shape and DeepSeek backends.
func NewRegistry() *Registry {
	oai := func(rt *RoundTrip, cc CatalogConfig) (LLM, error) {
		return openai.NewLLM(roundOrNil(rt).Client(), cc.BaseURL, cc.APIKey), nil
	}
	dsk := func(rt *RoundTrip, cc CatalogConfig) (LLM, error) {
		return deepseek.NewLLM(roundOrNil(rt).Client(), cc.BaseURL, cc.APIKey), nil
	}
	gmn := func(rt *RoundTrip, cc CatalogConfig) (LLM, error) {
		return gemini.NewLLM(context.Background(), roundOrNil(rt).Client(), cc.APIKey, cc.BaseURL, cc.ModelAPI)
	}
	m := map[string]LLMFactory{
		"":          oai,
		"openai":    oai,
		"groq":      oai,
		"azure":     oai,
		"azure_ad":  oai,
		"mistral":   oai,
		"together":  oai,
		"fireworks": oai,
		"custom":    oai,
		"deepseek":  dsk,
		"anthropic": oai,
		"gemini":    gmn,
		"google":    oai,
		"vertex":    oai,
	}
	return &Registry{byType: m, fallback: oai}
}

var defaultRegistry = NewRegistry()

// DefaultRegistry is the process-wide registry.
func DefaultRegistry() *Registry { return defaultRegistry }

// Register overrides a catalog provider_type binding.
func (r *Registry) Register(providerType string, factory LLMFactory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.byType == nil {
		r.byType = make(map[string]LLMFactory)
	}
	r.byType[strings.ToLower(strings.TrimSpace(providerType))] = factory
}

// Resolve returns model.LLM for cfg (Anthropic native host stays blocked).
func (r *Registry) Resolve(cc CatalogConfig, rt *RoundTrip) (LLM, error) {
	if IsLikelyAnthropicNativeAPI(cc.BaseURL, cc.ProviderType) {
		return nil, ErrAnthropicNativeEndpoint
	}
	key := strings.ToLower(strings.TrimSpace(cc.ProviderType))
	r.mu.RLock()
	defer r.mu.RUnlock()
	if f, ok := r.byType[key]; ok {
		return f(roundOrNil(rt), cc)
	}
	if r.fallback != nil {
		return r.fallback(roundOrNil(rt), cc)
	}
	return nil, errors.New("provider: no LLM factory registered")
}
