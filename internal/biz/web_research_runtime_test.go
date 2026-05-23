package biz

import "testing"

func TestApplyWebResearchEffectiveGate_missingKey(t *testing.T) {
	catalog := []Tool{{
		Key:         ToolKeyWebResearch,
		DisplayName: "Web 研究",
		Enabled:     true,
		Source:      "builtin",
	}}
	settings := AgentRuntimeSettings{ToolsEnabled: true, ToolsProfile: "research"}
	eff := buildAgentEffectiveTools(settings, catalog)
	applyWebResearchEffectiveGate(&eff, catalog, &WebResearchSetting{Provider: "tavily"}, nil)
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
	tool := Tool{Key: ToolKeyWebResearch, ConfigJSON: `{"provider":"tavily"}`}
	platform := WebResearchSetting{Provider: "tavily", HasAPIKey: true, APIKey: "secret"}
	if !webResearchConfigReady(tool, &platform) {
		t.Fatal("expected ready with platform api key")
	}
}
