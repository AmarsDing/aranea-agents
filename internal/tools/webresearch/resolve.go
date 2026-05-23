package webresearch

import (
	"strings"
	"time"
)

// MergePlatformConfig applies platform system_settings over per-agent tool config.
// Agent/tool override wins when already Ready(); otherwise platform API key and tuning apply.
func MergePlatformConfig(agent Config, platform PlatformFields) Config {
	out := agent
	if strings.TrimSpace(out.Provider) == "" {
		out.Provider = strings.TrimSpace(platform.Provider)
	}
	if !out.Ready() {
		if strings.TrimSpace(platform.APIKey) != "" {
			out.APIKey = strings.TrimSpace(platform.APIKey)
		}
	}
	if out.MaxResults <= 0 && platform.MaxResults > 0 {
		out.MaxResults = platform.MaxResults
	}
	if out.FetchTop <= 0 && platform.FetchTop > 0 {
		out.FetchTop = platform.FetchTop
	}
	if strings.TrimSpace(out.SearchDepth) == "" && strings.TrimSpace(platform.SearchDepth) != "" {
		out.SearchDepth = strings.TrimSpace(platform.SearchDepth)
	}
	if out.Timeout <= 0 && platform.TimeoutSec > 0 {
		out.Timeout = time.Duration(platform.TimeoutSec) * time.Second
	}
	if strings.TrimSpace(out.HTTPProxy) == "" && strings.TrimSpace(platform.HTTPProxy) != "" {
		out.HTTPProxy = strings.TrimSpace(platform.HTTPProxy)
	}
	applyConfigDefaults(&out)
	return out
}

// ResolveConfig merges agent tool config_json with optional platform fields and env fallbacks.
func ResolveConfig(agentMap map[string]any, platform *PlatformFields) Config {
	agent := ConfigFromMap(agentMap)
	if platform == nil {
		applyConfigDefaults(&agent)
		return agent
	}
	return MergePlatformConfig(agent, *platform)
}

// ResolveReady reports whether web_research can run with the given agent map and platform snapshot.
func ResolveReady(agentMap map[string]any, platform *PlatformFields) bool {
	return ResolveConfig(agentMap, platform).Ready()
}

// CatalogReady reports whether the tool catalog should show web_research as available.
// It treats a platform-stored key (HasAPIKey) as sufficient even when the key is not returned to callers.
func CatalogReady(agentMap map[string]any, platform *PlatformFields) bool {
	if ResolveReady(agentMap, platform) {
		return true
	}
	return PlatformHasAPIKey(platform)
}

func applyConfigDefaults(cfg *Config) {
	if cfg == nil {
		return
	}
	if cfg.MaxResults <= 0 {
		cfg.MaxResults = defaultMaxResults
	}
	if cfg.FetchTop <= 0 {
		cfg.FetchTop = defaultFetchTop
	}
	if strings.TrimSpace(cfg.Provider) == "" {
		cfg.Provider = ProviderTavily
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = defaultTimeoutSec * time.Second
	}
}
