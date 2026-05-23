package webresearch

import "testing"

func TestResolveConfig_platformKey(t *testing.T) {
	agent := map[string]any{"provider": "tavily"}
	platform := &PlatformFields{HasAPIKey: true, APIKey: "plat-key", Provider: "tavily", MaxResults: 5}
	cfg := ResolveConfig(agent, platform)
	if !cfg.Ready() || cfg.APIKey != "plat-key" {
		t.Fatalf("cfg=%+v", cfg)
	}
}

func TestCatalogReady_hasPlatformKeyOnly(t *testing.T) {
	platform := &PlatformFields{HasAPIKey: true, Provider: "tavily"}
	if !CatalogReady(map[string]any{"provider": "tavily"}, platform) {
		t.Fatal("expected catalog ready with platform HasAPIKey")
	}
}

func TestMergePlatformConfig_agentOverrideWins(t *testing.T) {
	agent := Config{Provider: ProviderTavily, APIKey: "agent-key"}
	platform := PlatformFields{APIKey: "plat-key", Provider: ProviderTavily}
	out := MergePlatformConfig(agent, platform)
	if out.APIKey != "agent-key" {
		t.Fatalf("api_key=%q", out.APIKey)
	}
}
