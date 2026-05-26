package agent

import (
	"testing"

	"aranea-agents/internal/biz"
)

func TestBuildPreviewReport_StaticSections(t *testing.T) {
	ag := biz.Agent{
		AgentKey:         "bot",
		DisplayName:      "Bot",
		AgentDescription: "helpful assistant",
		SystemPromptMode: "minimized",
		Files: []biz.AgentPromptFile{
			{Name: "AGENTS_CORE.md", Body: "core rules"},
			{Name: "RULE.md", Body: "be safe"},
		},
		Settings: &biz.AgentRuntimeSettings{
			ToolsEnabled:          true,
			IntentPassEnabled:     true,
			L2RecallEnabled:       true,
			SessionSummaryEnabled: true,
		},
	}
	report := BuildPreviewReport(t.Context(), ag, "minimized", Deps{})
	if report.Instruction == "" {
		t.Fatal("expected instruction")
	}
	if report.StaticTotalTokens <= 0 {
		t.Fatalf("static tokens: %d", report.StaticTotalTokens)
	}
	if report.RuntimeOverlayEst <= 0 {
		t.Fatalf("runtime overlay: %d", report.RuntimeOverlayEst)
	}
	foundRuntimeCue := false
	for _, s := range report.Sections {
		if s.Key == "runtime_cue" {
			foundRuntimeCue = true
		}
	}
	if !foundRuntimeCue {
		t.Fatalf("sections: %#v", report.Sections)
	}
}
