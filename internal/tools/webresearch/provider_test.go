package webresearch_test

import (
	"net/url"
	"strings"
	"testing"

	"aranea-agents/internal/tools/webresearch"
	"aranea-agents/pkg/loggateway"
)

func TestNewSearchProvider_tavily(t *testing.T) {
	cfg := webresearch.Config{Provider: "tavily", APIKey: "tvly-key"}
	p, err := webresearch.NewSearchProvider(cfg, loggateway.NewNoop())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p == nil {
		t.Fatal("expected non-nil provider")
	}
}

func TestNewSearchProvider_serpapi(t *testing.T) {
	cfg := webresearch.Config{Provider: "serpapi", APIKey: "serp-key"}
	p, err := webresearch.NewSearchProvider(cfg, loggateway.NewNoop())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p == nil {
		t.Fatal("expected non-nil provider")
	}
}

func TestNewSearchProvider_unsupported(t *testing.T) {
	cfg := webresearch.Config{Provider: "bing", APIKey: "key"}
	_, err := webresearch.NewSearchProvider(cfg, loggateway.NewNoop())
	if err == nil {
		t.Fatal("expected error for unsupported provider")
	}
	if !strings.Contains(err.Error(), "unsupported provider") {
		t.Fatalf("error = %q", err.Error())
	}
}

func TestNewSearchProvider_emptyProvider(t *testing.T) {
	cfg := webresearch.Config{Provider: "", APIKey: "key"}
	p, err := webresearch.NewSearchProvider(cfg, loggateway.NewNoop())
	if err != nil {
		t.Fatalf("empty provider should default to tavily: %v", err)
	}
	if p == nil {
		t.Fatal("expected non-nil provider")
	}
}

func TestNewTavilyProvider_valid(t *testing.T) {
	cfg := webresearch.Config{Provider: "tavily", APIKey: "tvly-key"}
	p, err := webresearch.NewTavilyProvider(cfg, loggateway.NewNoop())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p == nil {
		t.Fatal("expected non-nil provider")
	}
}

func TestNewTavilyProvider_emptyAPIKey(t *testing.T) {
	cfg := webresearch.Config{Provider: "tavily", APIKey: ""}
	_, err := webresearch.NewTavilyProvider(cfg, loggateway.NewNoop())
	if err == nil {
		t.Fatal("expected error for empty APIKey")
	}
	if !strings.Contains(err.Error(), "api_key is required") {
		t.Fatalf("error = %q", err.Error())
	}
}

func TestNewSerpAPIProvider_valid(t *testing.T) {
	cfg := webresearch.Config{Provider: "serpapi", APIKey: "serp-key"}
	p, err := webresearch.NewSerpAPIProvider(cfg, loggateway.NewNoop())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p == nil {
		t.Fatal("expected non-nil provider")
	}
}

func TestNewSerpAPIProvider_emptyAPIKey(t *testing.T) {
	cfg := webresearch.Config{Provider: "serpapi", APIKey: ""}
	_, err := webresearch.NewSerpAPIProvider(cfg, loggateway.NewNoop())
	if err == nil {
		t.Fatal("expected error for empty APIKey")
	}
	if !strings.Contains(err.Error(), "api_key is required") {
		t.Fatalf("error = %q", err.Error())
	}
}

func TestRedactedURL_withAPIKey(t *testing.T) {
	u, _ := url.Parse("https://serpapi.com/search.json?q=test&api_key=secret123")
	got := webresearch.RedactedURL(u)
	if strings.Contains(got, "secret123") {
		t.Fatalf("redacted URL should not contain api_key value: %q", got)
	}
	if !strings.Contains(got, "api_key=") {
		t.Fatalf("redacted URL should contain api_key param: %q", got)
	}
	parsed, _ := url.Parse(got)
	if parsed.Query().Get("api_key") != "***" {
		t.Fatalf("api_key query param should be ***: %q", parsed.Query().Get("api_key"))
	}
}

func TestRedactedURL_withoutAPIKey(t *testing.T) {
	u, _ := url.Parse("https://example.com/path?q=test")
	got := webresearch.RedactedURL(u)
	if got != u.String() {
		t.Fatalf("URL without api_key should be unchanged: %q", got)
	}
}

func TestRedactedURL_emptyURL(t *testing.T) {
	u, _ := url.Parse("")
	got := webresearch.RedactedURL(u)
	if got != "" {
		t.Fatalf("empty URL should produce empty result, got %q", got)
	}
}

func TestConfigFromSetting(t *testing.T) {
	cfg := webresearch.ConfigFromSetting("tavily", "my-key", 10, 3, "advanced", 30, "http://proxy")
	if cfg.Provider != "tavily" {
		t.Fatalf("Provider = %q", cfg.Provider)
	}
	if cfg.APIKey != "my-key" {
		t.Fatalf("APIKey = %q", cfg.APIKey)
	}
	if cfg.SearchDepth != "advanced" {
		t.Fatalf("SearchDepth = %q", cfg.SearchDepth)
	}
	if cfg.HTTPProxy != "http://proxy" {
		t.Fatalf("HTTPProxy = %q", cfg.HTTPProxy)
	}
	if cfg.MaxResults != 1 {
		t.Fatalf("MaxResults = %d, want 1 (overridden for test)", cfg.MaxResults)
	}
	if cfg.FetchTop != 0 {
		t.Fatalf("FetchTop = %d, want 0 (overridden for test)", cfg.FetchTop)
	}
	if cfg.IncludeAnswer {
		t.Fatal("IncludeAnswer should be false for test config")
	}
	if cfg.IncludeRawContent {
		t.Fatal("IncludeRawContent should be false for test config")
	}
}

func TestConfigFromSetting_zeroOptionals(t *testing.T) {
	cfg := webresearch.ConfigFromSetting("serpapi", "key", 0, 0, "", 0, "")
	if cfg.Provider != "serpapi" {
		t.Fatalf("Provider = %q", cfg.Provider)
	}
	if cfg.APIKey != "key" {
		t.Fatalf("APIKey = %q", cfg.APIKey)
	}
}
