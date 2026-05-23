package biz

import (
	"strings"

	"aranea-agents/internal/biz/tool"
)

// WebResearchSetting is now an alias for tool.WebResearchSetting.
type WebResearchSetting = tool.WebResearchSetting

// WebResearchConfigured reports whether platform settings can run web_research.
func WebResearchConfigured(s WebResearchSetting) bool {
	if !s.HasAPIKey && strings.TrimSpace(s.APIKey) == "" {
		return false
	}
	return strings.TrimSpace(s.Provider) != ""
}

// ApplyWebResearchPatch merges an update onto current settings.
func ApplyWebResearchPatch(cur WebResearchSetting, patch WebResearchSetting, updateAPIKey bool) WebResearchSetting {
	out := cur
	if p := strings.TrimSpace(patch.Provider); p != "" {
		out.Provider = p
	}
	if patch.MaxResults > 0 {
		out.MaxResults = patch.MaxResults
	}
	if patch.FetchTop > 0 {
		out.FetchTop = patch.FetchTop
	}
	if d := strings.TrimSpace(patch.SearchDepth); d != "" {
		out.SearchDepth = d
	}
	if patch.TimeoutSec > 0 {
		out.TimeoutSec = patch.TimeoutSec
	}
	out.HTTPProxy = strings.TrimSpace(patch.HTTPProxy)
	if updateAPIKey && strings.TrimSpace(patch.APIKey) != "" {
		out.APIKey = strings.TrimSpace(patch.APIKey)
		out.HasAPIKey = true
	}
	return out
}
