package service

import (
	"testing"

	"aranea-agents/internal/biz"
	webresearchpkg "aranea-agents/internal/tools/webresearch"
)

// Phase 1c-3: TestToEventPairs removed — it tested toEventPairs which lives in
// cmd/admin/wire.go behind a //go:build wireinject tag (not in package service).
// The test never compiled; trivial function doesn't warrant moving out of wire.

func TestWebResearchTesterAdapter_ConfigFromSetting(t *testing.T) {
	a := webResearchTesterAdapter{}
	cfg := a.ConfigFromSetting("tavily", "my-key", 10, 3, "advanced", 30, "http://proxy")
	if cfg.Provider != "tavily" {
		t.Fatalf("Provider = %q, want %q", cfg.Provider, "tavily")
	}
	if cfg.APIKey != "my-key" {
		t.Fatalf("APIKey = %q, want %q", cfg.APIKey, "my-key")
	}
	if cfg.MaxResults != 1 {
		t.Fatalf("MaxResults = %d, want 1 (ConfigFromSetting overrides for test)", cfg.MaxResults)
	}
	if cfg.FetchTop != 0 {
		t.Fatalf("FetchTop = %d, want 0 (ConfigFromSetting overrides for test)", cfg.FetchTop)
	}
	if cfg.SearchDepth != "advanced" {
		t.Fatalf("SearchDepth = %q, want %q", cfg.SearchDepth, "advanced")
	}
	if cfg.TimeoutSec != 30 {
		t.Fatalf("TimeoutSec = %d, want %d", cfg.TimeoutSec, 30)
	}
	if cfg.HTTPProxy != "http://proxy" {
		t.Fatalf("HTTPProxy = %q, want %q", cfg.HTTPProxy, "http://proxy")
	}
}

func TestWebResearchTesterAdapter_ConfigFromSetting_zeroOptionals(t *testing.T) {
	a := webResearchTesterAdapter{}
	cfg := a.ConfigFromSetting("serpapi", "key", 0, 0, "", 0, "")
	if cfg.Provider != "serpapi" {
		t.Fatalf("Provider = %q, want %q", cfg.Provider, "serpapi")
	}
	if cfg.APIKey != "key" {
		t.Fatalf("APIKey = %q, want %q", cfg.APIKey, "key")
	}
	if cfg.MaxResults != 1 {
		t.Fatalf("MaxResults = %d, want 1 (ConfigFromSetting overrides for test)", cfg.MaxResults)
	}
	if cfg.FetchTop != 0 {
		t.Fatalf("FetchTop = %d, want 0", cfg.FetchTop)
	}
	if cfg.SearchDepth != "basic" {
		t.Fatalf("SearchDepth = %q, want %q (default)", cfg.SearchDepth, "basic")
	}
	if cfg.TimeoutSec != 15 {
		t.Fatalf("TimeoutSec = %d, want 15 (default)", cfg.TimeoutSec)
	}
	if cfg.HTTPProxy != "" {
		t.Fatalf("HTTPProxy = %q, want empty", cfg.HTTPProxy)
	}
}

func TestWebResearchTesterAdapter_toWebresearchConfig(t *testing.T) {
	a := webResearchTesterAdapter{}
	cfg := biz.WebResearchTestConfig{
		Provider:    "tavily",
		APIKey:      "test-key",
		MaxResults:  8,
		FetchTop:    5,
		SearchDepth: "basic",
		TimeoutSec:  15,
		HTTPProxy:   "http://proxy:8080",
	}
	wc := a.toWebresearchConfig(cfg)
	if wc.Provider != "tavily" {
		t.Fatalf("Provider = %q, want %q", wc.Provider, "tavily")
	}
	if wc.APIKey != "test-key" {
		t.Fatalf("APIKey = %q, want %q", wc.APIKey, "test-key")
	}
	if wc.SearchDepth != "basic" {
		t.Fatalf("SearchDepth = %q, want %q", wc.SearchDepth, "basic")
	}
	if wc.HTTPProxy != "http://proxy:8080" {
		t.Fatalf("HTTPProxy = %q, want %q", wc.HTTPProxy, "http://proxy:8080")
	}
}

func TestWebResearchTesterAdapter_toWebresearchConfig_zeroValues(t *testing.T) {
	a := webResearchTesterAdapter{}
	cfg := biz.WebResearchTestConfig{
		Provider: "serpapi",
		APIKey:   "key",
	}
	wc := a.toWebresearchConfig(cfg)
	if wc.Provider != "serpapi" {
		t.Fatalf("Provider = %q, want %q", wc.Provider, "serpapi")
	}
	if wc.APIKey != "key" {
		t.Fatalf("APIKey = %q, want %q", wc.APIKey, "key")
	}
}

func TestWebResearchTesterAdapter_toWebresearchConfig_roundTrip(t *testing.T) {
	a := webResearchTesterAdapter{}
	bizCfg := a.ConfigFromSetting("tavily", "rt-key", 12, 4, "advanced", 20, "http://p")
	wc := a.toWebresearchConfig(bizCfg)
	if wc.Provider != "tavily" {
		t.Fatalf("Provider = %q", wc.Provider)
	}
	if wc.APIKey != "rt-key" {
		t.Fatalf("APIKey = %q", wc.APIKey)
	}
	if wc.SearchDepth != "advanced" {
		t.Fatalf("SearchDepth = %q", wc.SearchDepth)
	}
	if wc.HTTPProxy != "http://p" {
		t.Fatalf("HTTPProxy = %q", wc.HTTPProxy)
	}
}

var _ webresearchpkg.Config
