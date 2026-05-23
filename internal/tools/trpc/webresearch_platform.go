package trpc

import (
	"strings"

	"aranea-agents/internal/biz"
	webresearchpkg "aranea-agents/internal/tools/webresearch"
)

// MergeWebResearchConfig applies platform system_settings over per-agent tool config.
func MergeWebResearchConfig(agent webresearchpkg.Config, platform biz.WebResearchSetting) webresearchpkg.Config {
	return webresearchpkg.MergePlatformConfig(agent, PlatformFieldsFromBiz(platform))
}

// PlatformFieldsFromBiz maps biz settings to webresearch platform fields.
func PlatformFieldsFromBiz(platform biz.WebResearchSetting) webresearchpkg.PlatformFields {
	return webresearchpkg.PlatformFields{
		HasAPIKey:   platform.HasAPIKey,
		APIKey:      strings.TrimSpace(platform.APIKey),
		Provider:    strings.TrimSpace(platform.Provider),
		MaxResults:  platform.MaxResults,
		FetchTop:    platform.FetchTop,
		SearchDepth: strings.TrimSpace(platform.SearchDepth),
		TimeoutSec:  platform.TimeoutSec,
		HTTPProxy:   strings.TrimSpace(platform.HTTPProxy),
	}
}
