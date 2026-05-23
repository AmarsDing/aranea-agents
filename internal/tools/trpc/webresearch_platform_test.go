package trpc

import (
	"testing"
	"time"

	"aranea-agents/internal/biz"
	webresearchpkg "aranea-agents/internal/tools/webresearch"
)

func TestMergeWebResearchConfig_platformKey(t *testing.T) {
	agent := webresearchpkg.Config{Provider: "tavily"}
	platform := biz.WebResearchSetting{
		Provider:    "tavily",
		APIKey:      "tvly-x",
		MaxResults:  10,
		FetchTop:    4,
		SearchDepth: "advanced",
		TimeoutSec:  20,
	}
	out := MergeWebResearchConfig(agent, platform)
	if !out.Ready() || out.APIKey != "tvly-x" || out.MaxResults != 10 {
		t.Fatalf("got %#v", out)
	}
	if out.Timeout != 20*time.Second {
		t.Fatalf("timeout=%v", out.Timeout)
	}
}
