package biz

import "testing"

// stubWebResearchChecker is a test double for WebResearchReadinessChecker.
type stubWebResearchChecker struct {
	resolveReady bool
	catalogReady bool
}

func (s stubWebResearchChecker) ResolveReady(_ map[string]any, platform *WebResearchPlatformFields) bool {
	if platform != nil && (platform.HasAPIKey || platform.APIKey != "") {
		return true
	}
	return s.resolveReady
}

func (s stubWebResearchChecker) CatalogReady(agentMap map[string]any, platform *WebResearchPlatformFields) bool {
	if s.ResolveReady(agentMap, platform) {
		return true
	}
	return platform != nil && platform.HasAPIKey
}

func TestApplyWebResearchEffectiveGate_missingKey(t *testing.T) {
	catalog := []Tool{{
		Key:         ToolKeyWebResearch,
		DisplayName: "Web 研究",
		Enabled:     true,
		Source:      "builtin",
	}}
	settings := AgentRuntimeSettings{ToolsEnabled: true, ToolsProfile: "research"}
	eff := buildAgentEffectiveTools(settings, catalog)
	checker := stubWebResearchChecker{}
	applyWebResearchEffectiveGate(checker, &eff, catalog, &WebResearchSetting{Provider: "tavily"}, nil)
	var wr *EffectiveAgentTool
	for i := range eff.Items {
		if eff.Items[i].ToolKey == ToolKeyWebResearch {
			wr = &eff.Items[i]
			break
		}
	}
	if wr == nil {
		t.Fatal("web_research item missing")
	}
	if wr.Enabled || wr.Reason != "missing_api_key" {
		t.Fatalf("enabled=%v reason=%q", wr.Enabled, wr.Reason)
	}
}

func TestWebResearchConfigReady_platformKey(t *testing.T) {
	platform := WebResearchSetting{Provider: "tavily", HasAPIKey: true, APIKey: "secret"}
	checker := stubWebResearchChecker{}
	pf := webResearchPlatformFields(platform)
	if !checker.ResolveReady(map[string]any{"provider": "tavily"}, pf) {
		t.Fatal("expected ready with platform api key")
	}
}
