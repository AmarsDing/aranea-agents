package biz

import (
	"context"
	"strings"

	"aranea-agents/internal/biz/tool"
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
	IsReady(agentMap map[string]any, platform *WebResearchPlatformFields) bool
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

// bizToToolPlatformFields converts biz.WebResearchPlatformFields to tool.WebResearchPlatformFields.
func bizToToolPlatformFields(p *WebResearchPlatformFields) *tool.WebResearchPlatformFields {
	if p == nil {
		return nil
	}
	return &tool.WebResearchPlatformFields{
		HasAPIKey:   p.HasAPIKey,
		APIKey:      p.APIKey,
		Provider:    p.Provider,
		MaxResults:  p.MaxResults,
		FetchTop:    p.FetchTop,
		SearchDepth: p.SearchDepth,
		TimeoutSec:  p.TimeoutSec,
		HTTPProxy:   p.HTTPProxy,
	}
}

// checkerToReadinessFunc adapts a biz.WebResearchReadinessChecker to a tool.WebResearchReadinessFunc.
func checkerToReadinessFunc(c WebResearchReadinessChecker) tool.WebResearchReadinessFunc {
	if c == nil {
		return nil
	}
	return func(agentMap map[string]any, platform *tool.WebResearchPlatformFields) bool {
		return c.IsReady(agentMap, bizToToolPlatformFieldsReverse(platform))
	}
}

// bizToToolPlatformFieldsReverse converts tool.WebResearchPlatformFields to biz.WebResearchPlatformFields.
func bizToToolPlatformFieldsReverse(p *tool.WebResearchPlatformFields) *WebResearchPlatformFields {
	if p == nil {
		return nil
	}
	return &WebResearchPlatformFields{
		HasAPIKey:   p.HasAPIKey,
		APIKey:      p.APIKey,
		Provider:    p.Provider,
		MaxResults:  p.MaxResults,
		FetchTop:    p.FetchTop,
		SearchDepth: p.SearchDepth,
		TimeoutSec:  p.TimeoutSec,
		HTTPProxy:   p.HTTPProxy,
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
