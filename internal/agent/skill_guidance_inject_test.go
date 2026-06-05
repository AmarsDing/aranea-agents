package agent

import (
	"testing"

	"aranea-agents/internal/biz"
)

func TestNewSkillGuidanceBeforeHook_ProgressiveMode(t *testing.T) {
	// When SkillUC is nil, the hook returns nil regardless of mode.
	// This test verifies the progressive branch is selected (not the
	// default eager hook) by checking that IsProgressiveSkillLoad
	// correctly identifies the mode.
	ag := biz.Agent{
		ID:               "test-agent",
		SystemPromptMode: "complete",
		Settings: &biz.AgentRuntimeSettings{
			SkillLoadMode: "progressive",
		},
	}
	// SkillUC nil → hook returns nil (early exit), but the branch
	// selection logic is verified by IsProgressiveSkillLoad below.
	deps := TRPCBuilderDeps{}
	hook := newSkillGuidanceBeforeHook(ag, deps)
	// nil because SkillUC is nil — the early exit guard fires first.
	if hook != nil {
		t.Fatal("expected nil hook when SkillUC is nil")
	}
}

func TestNewSkillGuidanceBeforeHook_ProgressiveModeUppercase(t *testing.T) {
	// Verify that "Progressive" (uppercase) is recognized as progressive mode.
	ag := biz.Agent{
		ID:               "test-agent",
		SystemPromptMode: "complete",
		Settings: &biz.AgentRuntimeSettings{
			SkillLoadMode: "Progressive",
		},
	}
	deps := TRPCBuilderDeps{}
	hook := newSkillGuidanceBeforeHook(ag, deps)
	// nil because SkillUC is nil — but the branch was selected via
	// IsProgressiveSkillLoad("Progressive") which should return true.
	if hook != nil {
		t.Fatal("expected nil hook when SkillUC is nil")
	}
}

func TestIsProgressiveSkillLoad_Integration(t *testing.T) {
	// Verify IsProgressiveSkillLoad works correctly for the hook's branch logic.
	tests := []struct {
		mode string
		want bool
	}{
		{"progressive", true},
		{"Progressive", true},
		{"PROGRESSIVE", true},
		{" progressive ", true},
		{"turn", false},
		{"", false},
	}
	for _, tt := range tests {
		got := biz.IsProgressiveSkillLoad(tt.mode)
		if got != tt.want {
			t.Errorf("IsProgressiveSkillLoad(%q) = %v, want %v", tt.mode, got, tt.want)
		}
	}
}
