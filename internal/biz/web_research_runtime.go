package biz

import (
	"context"
	"strings"
)

// WebResearchPlatformFields holds platform-level defaults for web_research.
type WebResearchPlatformFields struct {
	HasAPIKey   bool
	APIKey      string
	Provider    string
	MaxResults  int
	FetchTop    int
	SearchDepth string
	TimeoutSec  int
	HTTPProxy   string
}

// WebResearchReadinessChecker abstracts web_research readiness resolution.
type WebResearchReadinessChecker interface {
	ResolveReady(agentMap map[string]any, platform *WebResearchPlatformFields) bool
	CatalogReady(agentMap map[string]any, platform *WebResearchPlatformFields) bool
}

// loadWebResearchPlatformFromSys loads WebResearchSetting via SystemSettingRepo.
func loadWebResearchPlatformFromSys(ctx context.Context, sys SystemSettingRepo) *WebResearchSetting {
	if sys == nil {
		return nil
	}
	s, err := sys.GetWebResearch(ctx)
	if err != nil {
		return nil
	}
	return &s
}

func webResearchPlatformFieldsPtr(s *WebResearchSetting) *WebResearchPlatformFields {
	if s == nil {
		return nil
	}
	p := webResearchPlatformFields(*s)
	return p
}

func webResearchPlatformFields(s WebResearchSetting) *WebResearchPlatformFields {
	if !s.HasAPIKey && strings.TrimSpace(s.APIKey) == "" && strings.TrimSpace(s.Provider) == "" {
		return nil
	}
	return &WebResearchPlatformFields{
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

func applyWebResearchEffectiveGate(checker WebResearchReadinessChecker, eff *AgentEffectiveTools, catalog []Tool, platform *WebResearchSetting, overrides []ToolAgentOverride) {
	if checker == nil {
		return
	}
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
		cfgMap := MergeToolConfigMaps(tool.ConfigJSON, tool.DefaultConfigJSON)
		if ov, ok := overrideByKey[ToolKeyWebResearch]; ok && ov != "" {
			MergeJSONMapInto(cfgMap, ov)
		}
		if checker.ResolveReady(cfgMap, webResearchPlatformFieldsPtr(platform)) {
			continue
		}
		eff.Items[i].Enabled = false
		eff.Items[i].EffectiveState = "denied"
		eff.Items[i].Reason = "missing_api_key"
	}
}
