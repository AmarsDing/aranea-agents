package webresearch_test

import (
	"testing"
	"time"

	"aranea-agents/internal/tools/webresearch"
)

func TestApplyConfigDefaults_zeroValues(t *testing.T) {
	cfg := &webresearch.Config{}
	webresearch.ApplyConfigDefaults(cfg)
	if cfg.MaxResults != 8 {
		t.Fatalf("MaxResults = %d, want 8", cfg.MaxResults)
	}
	if cfg.FetchTop != 5 {
		t.Fatalf("FetchTop = %d, want 5", cfg.FetchTop)
	}
	if cfg.Provider != "tavily" {
		t.Fatalf("Provider = %q, want tavily", cfg.Provider)
	}
	if cfg.Timeout != 15*time.Second {
		t.Fatalf("Timeout = %v, want 15s", cfg.Timeout)
	}
}

func TestApplyConfigDefaults_nonZeroPreserved(t *testing.T) {
	cfg := &webresearch.Config{
		Provider:   "serpapi",
		MaxResults: 3,
		FetchTop:   2,
		Timeout:    30 * time.Second,
	}
	webresearch.ApplyConfigDefaults(cfg)
	if cfg.MaxResults != 3 {
		t.Fatalf("MaxResults = %d, want 3", cfg.MaxResults)
	}
	if cfg.FetchTop != 2 {
		t.Fatalf("FetchTop = %d, want 2", cfg.FetchTop)
	}
	if cfg.Provider != "serpapi" {
		t.Fatalf("Provider = %q, want serpapi", cfg.Provider)
	}
	if cfg.Timeout != 30*time.Second {
		t.Fatalf("Timeout = %v, want 30s", cfg.Timeout)
	}
}

func TestApplyConfigDefaults_nil(t *testing.T) {
	webresearch.ApplyConfigDefaults(nil)
}

func TestMergePlatformConfig_emptyPlatform(t *testing.T) {
	agent := webresearch.Config{Provider: "tavily", APIKey: "agent-key"}
	platform := webresearch.PlatformFields{}
	out := webresearch.MergePlatformConfig(agent, platform)
	if out.APIKey != "agent-key" {
		t.Fatalf("APIKey = %q, want agent-key", out.APIKey)
	}
}

func TestMergePlatformConfig_platformProviderFills(t *testing.T) {
	agent := webresearch.Config{APIKey: "key"}
	platform := webresearch.PlatformFields{Provider: "serpapi"}
	out := webresearch.MergePlatformConfig(agent, platform)
	if out.Provider != "serpapi" {
		t.Fatalf("Provider = %q, want serpapi", out.Provider)
	}
}

func TestMergePlatformConfig_platformAPIKeyFallback(t *testing.T) {
	agent := webresearch.Config{Provider: "tavily"}
	platform := webresearch.PlatformFields{APIKey: "plat-key"}
	out := webresearch.MergePlatformConfig(agent, platform)
	if out.APIKey != "plat-key" {
		t.Fatalf("APIKey = %q, want plat-key", out.APIKey)
	}
}

func TestMergePlatformConfig_platformMaxResults(t *testing.T) {
	agent := webresearch.Config{Provider: "tavily", APIKey: "key"}
	platform := webresearch.PlatformFields{MaxResults: 20}
	out := webresearch.MergePlatformConfig(agent, platform)
	if out.MaxResults != 20 {
		t.Fatalf("MaxResults = %d, want 20", out.MaxResults)
	}
}

func TestMergePlatformConfig_platformFetchTop(t *testing.T) {
	agent := webresearch.Config{Provider: "tavily", APIKey: "key"}
	platform := webresearch.PlatformFields{FetchTop: 7}
	out := webresearch.MergePlatformConfig(agent, platform)
	if out.FetchTop != 7 {
		t.Fatalf("FetchTop = %d, want 7", out.FetchTop)
	}
}

func TestMergePlatformConfig_platformTimeout(t *testing.T) {
	agent := webresearch.Config{Provider: "tavily", APIKey: "key"}
	platform := webresearch.PlatformFields{TimeoutSec: 60}
	out := webresearch.MergePlatformConfig(agent, platform)
	if out.Timeout != 60*time.Second {
		t.Fatalf("Timeout = %v, want 60s", out.Timeout)
	}
}

func TestMergePlatformConfig_platformHTTPProxy(t *testing.T) {
	agent := webresearch.Config{Provider: "tavily", APIKey: "key"}
	platform := webresearch.PlatformFields{HTTPProxy: "http://proxy:3128"}
	out := webresearch.MergePlatformConfig(agent, platform)
	if out.HTTPProxy != "http://proxy:3128" {
		t.Fatalf("HTTPProxy = %q", out.HTTPProxy)
	}
}

func TestMergePlatformConfig_platformSearchDepth(t *testing.T) {
	agent := webresearch.Config{Provider: "tavily", APIKey: "key"}
	platform := webresearch.PlatformFields{SearchDepth: "advanced"}
	out := webresearch.MergePlatformConfig(agent, platform)
	if out.SearchDepth != "advanced" {
		t.Fatalf("SearchDepth = %q, want advanced", out.SearchDepth)
	}
}

func TestMergePlatformConfig_agentValuesWinOverPlatform(t *testing.T) {
	agent := webresearch.Config{
		Provider:    "tavily",
		APIKey:      "agent-key",
		MaxResults:  3,
		FetchTop:    2,
		SearchDepth: "basic",
		Timeout:     10 * time.Second,
		HTTPProxy:   "http://agent-proxy",
	}
	platform := webresearch.PlatformFields{
		APIKey:      "plat-key",
		MaxResults:  20,
		FetchTop:    7,
		SearchDepth: "advanced",
		TimeoutSec:  60,
		HTTPProxy:   "http://plat-proxy",
	}
	out := webresearch.MergePlatformConfig(agent, platform)
	if out.APIKey != "agent-key" {
		t.Fatalf("APIKey = %q, agent should win", out.APIKey)
	}
	if out.MaxResults != 3 {
		t.Fatalf("MaxResults = %d, agent should win", out.MaxResults)
	}
	if out.FetchTop != 2 {
		t.Fatalf("FetchTop = %d, agent should win", out.FetchTop)
	}
	if out.SearchDepth != "basic" {
		t.Fatalf("SearchDepth = %q, agent should win", out.SearchDepth)
	}
	if out.Timeout != 10*time.Second {
		t.Fatalf("Timeout = %v, agent should win", out.Timeout)
	}
	if out.HTTPProxy != "http://agent-proxy" {
		t.Fatalf("HTTPProxy = %q, agent should win", out.HTTPProxy)
	}
}

func TestCatalogReady_resolveReadyTrue(t *testing.T) {
	if !webresearch.CatalogReady(map[string]any{"api_key": "k"}, nil) {
		t.Fatal("expected catalog ready when resolve is ready")
	}
}

func TestCatalogReady_platformNil(t *testing.T) {
	if webresearch.CatalogReady(nil, nil) {
		t.Fatal("expected catalog not ready with nil platform and no key")
	}
}

func TestCatalogReady_platformHasAPIKey(t *testing.T) {
	platform := &webresearch.PlatformFields{HasAPIKey: true}
	if !webresearch.CatalogReady(nil, platform) {
		t.Fatal("expected catalog ready with platform HasAPIKey")
	}
}

func TestPlatformHasAPIKey_nil(t *testing.T) {
	if webresearch.PlatformHasAPIKey(nil) {
		t.Fatal("expected false for nil platform")
	}
}

func TestPlatformHasAPIKey_emptyAPIKey(t *testing.T) {
	p := &webresearch.PlatformFields{}
	if webresearch.PlatformHasAPIKey(p) {
		t.Fatal("expected false for empty APIKey and HasAPIKey=false")
	}
}

func TestPlatformHasAPIKey_hasAPIKeyTrueButEmptyAPIKey(t *testing.T) {
	p := &webresearch.PlatformFields{HasAPIKey: true}
	if !webresearch.PlatformHasAPIKey(p) {
		t.Fatal("expected true when HasAPIKey is true")
	}
}

func TestPlatformHasAPIKey_validAPIKey(t *testing.T) {
	p := &webresearch.PlatformFields{APIKey: "real-key"}
	if !webresearch.PlatformHasAPIKey(p) {
		t.Fatal("expected true when APIKey is set")
	}
}

func TestPlatformHasAPIKey_whitespaceAPIKey(t *testing.T) {
	p := &webresearch.PlatformFields{APIKey: "   "}
	if webresearch.PlatformHasAPIKey(p) {
		t.Fatal("expected false for whitespace-only APIKey")
	}
}
