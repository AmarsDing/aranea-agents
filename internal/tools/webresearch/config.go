package webresearch

import (
	"os"
	"strings"
	"time"

	"aranea-agents/internal/tools"
)

// Config holds platform/agent configuration for web_research.
type Config struct {
	Provider          string
	APIKey            string
	SearchDepth       string
	MaxResults        int
	FetchTop          int
	IncludeAnswer     bool
	IncludeRawContent bool
	Timeout           time.Duration
	HTTPProxy         string
	// TavilySearchURL overrides the Tavily API endpoint (tests only).
	TavilySearchURL string
}

const (
	ProviderTavily  = "tavily"
	ProviderSerpAPI = "serpapi"

	defaultMaxResults = 8
	defaultFetchTop   = 5
	defaultTimeoutSec = 15
)

// ConfigFromMap builds Config from merged tool config_json (catalog + agent override).
func ConfigFromMap(m map[string]any) Config {
	cfg := Config{
		Provider:          ProviderTavily,
		SearchDepth:       "basic",
		MaxResults:        defaultMaxResults,
		FetchTop:          defaultFetchTop,
		IncludeAnswer:     true,
		IncludeRawContent: true,
		Timeout:           defaultTimeoutSec * time.Second,
	}
	if len(m) == 0 {
		cfg.APIKey = resolveAPIKeyFromEnv(cfg.Provider)
		return cfg
	}
	if v := tools.ConfigString(m, "provider", "search_provider"); v != "" {
		cfg.Provider = strings.ToLower(v)
	}
	cfg.APIKey = tools.ConfigString(m, "api_key", "tavily_api_key", "serpapi_api_key")
	if cfg.APIKey == "" {
		cfg.APIKey = resolveAPIKeyFromEnv(cfg.Provider)
	}
	if v := tools.ConfigString(m, "search_depth"); v != "" {
		cfg.SearchDepth = v
	}
	if n := configInt(m, "max_results"); n > 0 {
		cfg.MaxResults = n
	}
	if n := configInt(m, "fetch_top"); n > 0 {
		cfg.FetchTop = n
	}
	if _, ok := m["include_answer"]; ok {
		cfg.IncludeAnswer = configBool(m, "include_answer")
	}
	if _, ok := m["include_raw_content"]; ok {
		cfg.IncludeRawContent = configBool(m, "include_raw_content")
	}
	if sec := configInt(m, "timeout_sec", "timeout_seconds"); sec > 0 {
		cfg.Timeout = time.Duration(sec) * time.Second
	}
	cfg.HTTPProxy = tools.ConfigString(m, "http_proxy", "proxy_url")
	if cfg.HTTPProxy == "" {
		cfg.HTTPProxy = strings.TrimSpace(os.Getenv("ARANEA_WEB_HTTP_PROXY"))
	}
	return cfg
}

func (c Config) Ready() bool {
	return strings.TrimSpace(c.APIKey) != ""
}

func resolveAPIKeyFromEnv(provider string) string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case ProviderSerpAPI:
		return strings.TrimSpace(os.Getenv("SERPAPI_API_KEY"))
	default:
		return strings.TrimSpace(os.Getenv("TAVILY_API_KEY"))
	}
}

func configInt(m map[string]any, keys ...string) int {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			switch n := v.(type) {
			case float64:
				if n > 0 {
					return int(n)
				}
			case int:
				if n > 0 {
					return n
				}
			}
		}
	}
	return 0
}

func configBool(m map[string]any, key string) bool {
	v, ok := m[key]
	if !ok {
		return false
	}
	switch t := v.(type) {
	case bool:
		return t
	case float64:
		return t != 0
	default:
		return false
	}
}
