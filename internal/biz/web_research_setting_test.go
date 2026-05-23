package biz

import "testing"

func TestWebResearchConfigured(t *testing.T) {
	if WebResearchConfigured(WebResearchSetting{Provider: "tavily", HasAPIKey: true}) != true {
		t.Fatal("expected configured")
	}
	if WebResearchConfigured(WebResearchSetting{Provider: "tavily"}) != false {
		t.Fatal("expected not configured without key")
	}
}

func TestApplyWebResearchPatch_keepsKeyUnlessUpdate(t *testing.T) {
	cur := WebResearchSetting{Provider: "tavily", HasAPIKey: true, APIKey: "secret", MaxResults: 8}
	out := ApplyWebResearchPatch(cur, WebResearchSetting{MaxResults: 10}, false)
	if out.APIKey != "secret" || out.MaxResults != 10 {
		t.Fatalf("got %#v", out)
	}
}
