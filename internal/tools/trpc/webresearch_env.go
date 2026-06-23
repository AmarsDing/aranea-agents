package trpc

import (
	"os"
	"strings"

	webresearchpkg "aranea-agents/internal/tools/webresearch"
)

// WebResearchConfigFromEnv fills API key from TAVILY_API_KEY / SERPAPI_API_KEY
// when the provided config is not ready (APIKey empty). Unlike ConfigFromMap(nil),
// this preserves all other fields of cfg (Provider, SearchDepth, MaxResults, etc.)
// and only resolves the APIKey from the environment variable matching cfg.Provider.
func WebResearchConfigFromEnv(cfg webresearchpkg.Config) webresearchpkg.Config {
	if cfg.Ready() {
		return cfg
	}
	provider := strings.ToLower(strings.TrimSpace(cfg.Provider))
	if provider == "" {
		provider = webresearchpkg.ProviderTavily
	}
	switch provider {
	case webresearchpkg.ProviderSerpAPI:
		cfg.APIKey = strings.TrimSpace(os.Getenv("SERPAPI_API_KEY"))
	case webresearchpkg.ProviderTavily, "":
		cfg.APIKey = strings.TrimSpace(os.Getenv("TAVILY_API_KEY"))
	}
	return cfg
}
