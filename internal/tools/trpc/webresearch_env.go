package trpc

import webresearchpkg "aranea-agents/internal/tools/webresearch"

// WebResearchConfigFromEnv fills API key from TAVILY_API_KEY / SERPAPI_API_KEY when config map is empty.
func WebResearchConfigFromEnv(cfg webresearchpkg.Config) webresearchpkg.Config {
	if cfg.Ready() {
		return cfg
	}
	return webresearchpkg.ConfigFromMap(nil)
}
