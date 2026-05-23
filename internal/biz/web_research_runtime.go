package biz

import (
	"strings"

	webresearchpkg "aranea-agents/internal/tools/webresearch"
)

func webResearchPlatformFieldsPtr(s *WebResearchSetting) *webresearchpkg.PlatformFields {
	if s == nil {
		return nil
	}
	p := webResearchPlatformFields(*s)
	return p
}

func webResearchPlatformFields(s WebResearchSetting) *webresearchpkg.PlatformFields {
	if !s.HasAPIKey && strings.TrimSpace(s.APIKey) == "" && strings.TrimSpace(s.Provider) == "" {
		return nil
	}
	return &webresearchpkg.PlatformFields{
		HasAPIKey:   s.HasAPIKey,
		APIKey:      s.APIKey,
		Provider:    s.Provider,
		MaxResults:  s.MaxResults,
		FetchTop:    s.FetchTop,
		SearchDepth: s.SearchDepth,
		TimeoutSec:  s.TimeoutSec,
		HTTPProxy:   s.HTTPProxy,
	}
}

func webResearchConfigReady(tool Tool, platform *WebResearchSetting) bool {
	m := mergeToolConfigMaps(tool.ConfigJSON, tool.DefaultConfigJSON)
	var pf *webresearchpkg.PlatformFields
	if platform != nil {
		pf = webResearchPlatformFields(*platform)
	}
	return webresearchpkg.ResolveReady(m, pf)
}

func applyWebResearchEffectiveGate(eff *AgentEffectiveTools, catalog []Tool, platform *WebResearchSetting, overrides []ToolAgentOverride) {
	if eff == nil || platform == nil {
		return
	}
	overrideByKey := make(map[string]string, len(overrides))
	for _, o := range overrides {
		overrideByKey[o.ToolKey] = o.ConfigOverrideJSON
	}
	catalogByKey := make(map[string]Tool, len(catalog))
	for _, t := range catalog {
		catalogByKey[t.Key] = t
	}
	for i := range eff.Items {
		if eff.Items[i].ToolKey != ToolKeyWebResearch {
			continue
		}
		if eff.Items[i].EffectiveState != "allowed" {
			continue
		}
		tool, ok := catalogByKey[ToolKeyWebResearch]
		if !ok {
			continue
		}
		cfgMap := mergeToolConfigMaps(tool.ConfigJSON, tool.DefaultConfigJSON)
		if ov, ok := overrideByKey[ToolKeyWebResearch]; ok && ov != "" {
			mergeJSONMapInto(cfgMap, ov)
		}
		if webresearchpkg.ResolveReady(cfgMap, webResearchPlatformFieldsPtr(platform)) {
			continue
		}
		eff.Items[i].Enabled = false
		eff.Items[i].EffectiveState = "denied"
		eff.Items[i].Reason = "missing_api_key"
	}
}
