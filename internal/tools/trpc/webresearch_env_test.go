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

func TestWebResearchConfigFromEnv_serpapiEnvNotReady(t *testing.T) {
	t.Setenv("SERPAPI_API_KEY", "serp-from-env")
	cfg := webresearchpkg.Config{Provider: "serpapi"}
	out := trpc.WebResearchConfigFromEnv(cfg)
	if out.Ready() {
		t.Fatal("ConfigFromMap(nil) defaults to tavily, so SERPAPI_API_KEY alone should not make config ready")
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
