package webresearch_test

import (
	"testing"

	"aranea-agents/internal/tools/webresearch"
)

func TestResolveReady_withAPIKey(t *testing.T) {
	if !webresearch.ResolveReady(map[string]any{"api_key": "k"}, nil) {
		t.Fatal("expected ready with api_key")
	}
}

func TestResolveReady_noKey(t *testing.T) {
	if webresearch.ResolveReady(nil, nil) {
		t.Fatal("expected not ready without key")
	}
}

func TestResolveReady_platformKey(t *testing.T) {
	platform := &webresearch.PlatformFields{APIKey: "plat-key", Provider: "tavily"}
	if !webresearch.ResolveReady(nil, platform) {
		t.Fatal("expected ready with platform key")
	}
}

func TestResolveReady_agentKeyOverridesPlatform(t *testing.T) {
	platform := &webresearch.PlatformFields{APIKey: "plat-key"}
	if !webresearch.ResolveReady(map[string]any{"api_key": "agent-key"}, platform) {
		t.Fatal("expected ready with agent key")
	}
}

func TestResolveConfig_nilPlatform(t *testing.T) {
	cfg := webresearch.ResolveConfig(map[string]any{"api_key": "k"}, nil)
	if !cfg.Ready() {
		t.Fatal("expected ready config")
	}
	if cfg.APIKey != "k" {
		t.Fatalf("APIKey = %q, want k", cfg.APIKey)
	}
}

func TestResolveConfig_emptyMapNilPlatform(t *testing.T) {
	cfg := webresearch.ResolveConfig(nil, nil)
	if cfg.Ready() {
		t.Fatal("expected not ready with empty map and nil platform")
	}
}

func TestResolveConfig_platformFields(t *testing.T) {
	platform := &webresearch.PlatformFields{
		APIKey:    "plat-key",
		HTTPProxy: "http://plat-proxy:3128",
	}
	cfg := webresearch.ResolveConfig(nil, platform)
	if !cfg.Ready() {
		t.Fatal("expected ready with platform key")
	}
	if cfg.HTTPProxy != "http://plat-proxy:3128" {
		t.Fatalf("HTTPProxy = %q, want http://plat-proxy:3128", cfg.HTTPProxy)
	}
}

func TestNewTool_notReady(t *testing.T) {
	_, err := webresearch.NewTool(webresearch.Config{}, nil)
	if err == nil {
		t.Fatal("expected error when config not ready")
	}
}

func TestNewTool_unsupportedProvider(t *testing.T) {
	_, err := webresearch.NewTool(webresearch.Config{
		Provider: "bing",
		APIKey:   "key",
	}, nil)
	if err == nil {
		t.Fatal("expected error for unsupported provider")
	}
}

func TestNewTool_validTavily(t *testing.T) {
	tool, err := webresearch.NewTool(webresearch.Config{
		Provider:   "tavily",
		APIKey:     "test-key",
		MaxResults: 5,
		FetchTop:   0,
	}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tool == nil {
		t.Fatal("expected non-nil tool")
	}
	d := tool.Declaration()
	if d == nil {
		t.Fatal("expected non-nil declaration")
	}
	if d.Name != "web_research" {
		t.Fatalf("Name = %q, want web_research", d.Name)
	}
}

func TestNewTool_validSerpAPI(t *testing.T) {
	tool, err := webresearch.NewTool(webresearch.Config{
		Provider:   "serpapi",
		APIKey:     "test-key",
		MaxResults: 5,
		FetchTop:   0,
	}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tool == nil {
		t.Fatal("expected non-nil tool")
	}
}

func TestConfigFromMap_serpapiEnvFallback(t *testing.T) {
	t.Setenv("SERPAPI_API_KEY", "serp-env-key")
	cfg := webresearch.ConfigFromMap(map[string]any{"provider": "serpapi"})
	if cfg.APIKey != "serp-env-key" {
		t.Fatalf("APIKey = %q, want serp-env-key", cfg.APIKey)
	}
	if cfg.Provider != "serpapi" {
		t.Fatalf("Provider = %q, want serpapi", cfg.Provider)
	}
}

func TestConfigFromMap_providerCaseInsensitive(t *testing.T) {
	cfg := webresearch.ConfigFromMap(map[string]any{
		"provider": "Tavily",
		"api_key":  "k",
	})
	if cfg.Provider != "tavily" {
		t.Fatalf("Provider = %q, want tavily (lowercased)", cfg.Provider)
	}
}

func TestConfigFromMap_httpProxyEnv(t *testing.T) {
	t.Setenv("ARANEA_WEB_HTTP_PROXY", "http://env-proxy:3128")
	cfg := webresearch.ConfigFromMap(map[string]any{"api_key": "k"})
	if cfg.HTTPProxy != "http://env-proxy:3128" {
		t.Fatalf("HTTPProxy = %q, want http://env-proxy:3128", cfg.HTTPProxy)
	}
}

func TestConfigFromMap_httpProxyConfigOverridesEnv(t *testing.T) {
	t.Setenv("ARANEA_WEB_HTTP_PROXY", "http://env-proxy:3128")
	cfg := webresearch.ConfigFromMap(map[string]any{
		"api_key":    "k",
		"http_proxy": "http://config-proxy:3128",
	})
	if cfg.HTTPProxy != "http://config-proxy:3128" {
		t.Fatalf("HTTPProxy = %q, want config value", cfg.HTTPProxy)
	}
}

func TestConfigFromMap_searchDepthOverride(t *testing.T) {
	cfg := webresearch.ConfigFromMap(map[string]any{
		"api_key":      "k",
		"search_depth": "advanced",
	})
	if cfg.SearchDepth != "advanced" {
		t.Fatalf("SearchDepth = %q, want advanced", cfg.SearchDepth)
	}
}

func TestConfigFromMap_timeoutSecondsAlias(t *testing.T) {
	cfg := webresearch.ConfigFromMap(map[string]any{
		"api_key":         "k",
		"timeout_seconds": float64(45),
	})
	if cfg.Timeout.Seconds() != 45 {
		t.Fatalf("Timeout = %v, want 45s", cfg.Timeout)
	}
}

func TestConfigFromMap_includeAnswerFalse(t *testing.T) {
	cfg := webresearch.ConfigFromMap(map[string]any{
		"api_key":        "k",
		"include_answer": false,
	})
	if cfg.IncludeAnswer {
		t.Fatal("IncludeAnswer should be false when explicitly set")
	}
}

func TestConfigFromMap_includeRawContentFalse(t *testing.T) {
	cfg := webresearch.ConfigFromMap(map[string]any{
		"api_key":             "k",
		"include_raw_content": false,
	})
	if cfg.IncludeRawContent {
		t.Fatal("IncludeRawContent should be false when explicitly set")
	}
}

func TestConfigFromMap_tavilyApiKeyAlias(t *testing.T) {
	cfg := webresearch.ConfigFromMap(map[string]any{
		"tavily_api_key": "tvly-key",
	})
	if cfg.APIKey != "tvly-key" {
		t.Fatalf("APIKey = %q, want tvly-key", cfg.APIKey)
	}
}

func TestConfigFromMap_serpapiApiKeyAlias(t *testing.T) {
	cfg := webresearch.ConfigFromMap(map[string]any{
		"serpapi_api_key": "serp-key",
	})
	if cfg.APIKey != "serp-key" {
		t.Fatalf("APIKey = %q, want serp-key", cfg.APIKey)
	}
}

func TestConfigFromMap_searchProviderAlias(t *testing.T) {
	cfg := webresearch.ConfigFromMap(map[string]any{
		"search_provider": "SerpAPI",
		"api_key":         "k",
	})
	if cfg.Provider != "serpapi" {
		t.Fatalf("Provider = %q, want serpapi", cfg.Provider)
	}
}

func TestConfigFromMap_proxyUrlAlias(t *testing.T) {
	cfg := webresearch.ConfigFromMap(map[string]any{
		"api_key":   "k",
		"proxy_url": "http://proxy:8080",
	})
	if cfg.HTTPProxy != "http://proxy:8080" {
		t.Fatalf("HTTPProxy = %q, want http://proxy:8080", cfg.HTTPProxy)
	}
}
