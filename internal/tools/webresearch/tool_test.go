package webresearch

import (
	"context"
	"testing"
)

func TestResearchTool_Call_requiresQuery(t *testing.T) {
	tool, err := NewTool(Config{Provider: ProviderTavily, APIKey: "k"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tool.Call(context.Background(), []byte(`{}`))
	if err == nil {
		t.Fatal("expected error for empty query")
	}
}

func TestConfigFromMap_envFallback(t *testing.T) {
	t.Setenv("TAVILY_API_KEY", "from-env")
	cfg := ConfigFromMap(map[string]any{"provider": "tavily"})
	if cfg.APIKey != "from-env" {
		t.Fatalf("api_key=%q", cfg.APIKey)
	}
	if !cfg.Ready() {
		t.Fatal("expected ready")
	}
}

func TestConfig_Ready_serpapi(t *testing.T) {
	t.Setenv("SERPAPI_API_KEY", "serp-key")
	cfg := ConfigFromMap(map[string]any{"provider": "serpapi"})
	if cfg.APIKey != "serp-key" {
		t.Fatalf("api_key=%q", cfg.APIKey)
	}
}
