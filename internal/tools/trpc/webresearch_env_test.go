package trpc_test

import (
	"testing"

	"aranea-agents/internal/tools/trpc"
	webresearchpkg "aranea-agents/internal/tools/webresearch"
)

func TestWebResearchConfigFromEnv_readyCfg(t *testing.T) {
	cfg := webresearchpkg.Config{Provider: "tavily", APIKey: "existing-key"}
	out := trpc.WebResearchConfigFromEnv(cfg)
	if out.APIKey != "existing-key" {
		t.Fatalf("APIKey = %q, want %q", out.APIKey, "existing-key")
	}
}

func TestWebResearchConfigFromEnv_tavilyEnvFallback(t *testing.T) {
	t.Setenv("TAVILY_API_KEY", "tvly-from-env")
	cfg := webresearchpkg.Config{Provider: "tavily"}
	out := trpc.WebResearchConfigFromEnv(cfg)
	if !out.Ready() {
		t.Fatal("expected ready after TAVILY_API_KEY env")
	}
	if out.APIKey != "tvly-from-env" {
		t.Fatalf("APIKey = %q, want %q", out.APIKey, "tvly-from-env")
	}
}

func TestWebResearchConfigFromEnv_serpapiEnvReady(t *testing.T) {
	t.Setenv("SERPAPI_API_KEY", "serp-from-env")
	cfg := webresearchpkg.Config{Provider: "serpapi"}
	out := trpc.WebResearchConfigFromEnv(cfg)
	if !out.Ready() {
		t.Fatal("expected ready after SERPAPI_API_KEY env with serpapi provider")
	}
	if out.APIKey != "serp-from-env" {
		t.Fatalf("APIKey = %q, want %q", out.APIKey, "serp-from-env")
	}
	if out.Provider != "serpapi" {
		t.Fatalf("Provider = %q, want serpapi (must be preserved)", out.Provider)
	}
}

func TestWebResearchConfigFromEnv_serpapiIgnoresTavilyEnv(t *testing.T) {
	// When Provider is serpapi, TAVILY_API_KEY must NOT be read.
	t.Setenv("TAVILY_API_KEY", "tvly-from-env")
	cfg := webresearchpkg.Config{Provider: "serpapi"}
	out := trpc.WebResearchConfigFromEnv(cfg)
	if out.Ready() {
		t.Fatal("serpapi provider must not become ready from TAVILY_API_KEY env")
	}
}

func TestWebResearchConfigFromEnv_preservesOtherFields(t *testing.T) {
	t.Setenv("TAVILY_API_KEY", "tvly-from-env")
	cfg := webresearchpkg.Config{
		Provider:      "tavily",
		SearchDepth:   "advanced",
		MaxResults:    15,
		FetchTop:      3,
		IncludeAnswer: false,
	}
	out := trpc.WebResearchConfigFromEnv(cfg)
	if out.SearchDepth != "advanced" {
		t.Fatalf("SearchDepth = %q, want advanced (must be preserved)", out.SearchDepth)
	}
	if out.MaxResults != 15 {
		t.Fatalf("MaxResults = %d, want 15 (must be preserved)", out.MaxResults)
	}
	if out.FetchTop != 3 {
		t.Fatalf("FetchTop = %d, want 3 (must be preserved)", out.FetchTop)
	}
	if out.IncludeAnswer != false {
		t.Fatalf("IncludeAnswer = %v, want false (must be preserved)", out.IncludeAnswer)
	}
}

func TestWebResearchConfigFromEnv_serpapiReadyCfg(t *testing.T) {
	cfg := webresearchpkg.Config{Provider: "serpapi", APIKey: "serp-existing"}
	out := trpc.WebResearchConfigFromEnv(cfg)
	if out.APIKey != "serp-existing" {
		t.Fatalf("APIKey = %q, want %q", out.APIKey, "serp-existing")
	}
	if out.Provider != "serpapi" {
		t.Fatalf("Provider = %q, want serpapi", out.Provider)
	}
}

func TestWebResearchConfigFromEnv_noEnvVars(t *testing.T) {
	cfg := webresearchpkg.Config{Provider: "tavily"}
	out := trpc.WebResearchConfigFromEnv(cfg)
	if out.Ready() {
		t.Fatal("expected not ready without env vars")
	}
}
