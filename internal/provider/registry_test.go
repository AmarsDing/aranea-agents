package provider_test

import (
	"strings"
	"testing"

	"aranea-agents/internal/provider"
)

func TestRegistryResolve_anthropicNative(t *testing.T) {
	r := provider.NewRegistry()
	_, err := r.Resolve(provider.CatalogConfig{
		ProviderType: "anthropic",
		BaseURL:      "https://api.anthropic.com",
		APIKey:       "k",
	}, &provider.RoundTrip{})
	if err != provider.ErrAnthropicNativeEndpoint {
		t.Fatalf("want ErrAnthropicNativeEndpoint, got %v", err)
	}
}

func TestRegistryResolve_OpenRouterAnthropicCompat(t *testing.T) {
	r := provider.NewRegistry()
	llm, err := r.Resolve(provider.CatalogConfig{
		ProviderType: "anthropic",
		BaseURL:      "https://openrouter.ai/api/v1",
		APIKey:       "k",
	}, &provider.RoundTrip{})
	if err != nil {
		t.Fatal(err)
	}
	if llm == nil {
		t.Fatal("nil llm")
	}
}

func TestMergeCatalogConfig_trimsBaseURL(t *testing.T) {
	cfg := provider.MergeCatalogConfig(provider.CatalogConfig{}, `{"api_base_url":" https://x/ "}`)
	if !strings.Contains(strings.TrimSpace(cfg.BaseURL), "https://x") {
		t.Fatalf("base URL: %q", cfg.BaseURL)
	}
}
