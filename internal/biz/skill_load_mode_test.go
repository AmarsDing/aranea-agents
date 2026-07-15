package biz

import "testing"

func TestIsProgressiveSkillLoad(t *testing.T) {
	tests := []struct {
		mode string
		want bool
	}{
		{"progressive", true},
		{"Progressive", true},
		{"PROGRESSIVE", true},
		{" progressive ", true},
		{" Progressive ", true},
		{"turn", false},
		{"eager", false},
		{"", false},
		{"progress", false},
	}
	for _, tt := range tests {
		got := IsProgressiveSkillLoad(tt.mode)
		if got != tt.want {
			t.Errorf("IsProgressiveSkillLoad(%q) = %v, want %v", tt.mode, got, tt.want)
		}
	}
}

func TestGetSkillLoadMode_DefaultEmpty(t *testing.T) {
	// nil settings returns empty string
	var s *AgentRuntimeSettings
	if got := s.GetSkillLoadMode(); got != "" {
		t.Errorf("GetSkillLoadMode(nil) = %q, want empty string", got)
	}
	// empty SkillLoadMode returns empty string
	s2 := &AgentRuntimeSettings{}
	if got := s2.GetSkillLoadMode(); got != "" {
		t.Errorf("GetSkillLoadMode(empty) = %q, want empty string", got)
	}
	// explicit mode returned as-is
	s3 := &AgentRuntimeSettings{SkillLoadMode: "progressive"}
	if got := s3.GetSkillLoadMode(); got != "progressive" {
		t.Errorf("GetSkillLoadMode(progressive) = %q, want progressive", got)
	}
}

// TestSkillLoadMode_P2_01_ProgressiveNotPassedToFramework verifies the
// P2-01 contract: "progressive" is an Aranea-specific composite marker
// that must NOT be passed to the framework's WithSkillLoadMode. The
// framework only recognizes "once|turn|session" and silently falls back
// to "turn" for unknown values, which would mask the composite semantic.
//
// This test mirrors the guard in trpc_build.go (line ~403) to ensure the
// exclusion predicate is stable.
func TestSkillLoadMode_P2_01_ProgressiveNotPassedToFramework(t *testing.T) {
	// Modes that SHOULD be passed to WithSkillLoadMode (framework recognizes them).
	passThrough := []string{"once", "turn", "session"}
	for _, mode := range passThrough {
		if IsProgressiveSkillLoad(mode) {
			t.Errorf("mode %q should not be flagged as progressive (would incorrectly block pass-through)", mode)
		}
	}
	// "progressive" (and case variants) MUST be blocked from pass-through.
	if !IsProgressiveSkillLoad("progressive") {
		t.Error("progressive must be flagged as progressive so it is excluded from WithSkillLoadMode")
	}
	if !IsProgressiveSkillLoad("Progressive") {
		t.Error("Progressive (uppercase) must be flagged as progressive")
	}
	// "auto" is excluded by a separate guard in trpc_build.go; verify it
	// is NOT flagged as progressive (it has its own exclusion condition).
	if IsProgressiveSkillLoad("auto") {
		t.Error("auto must not be flagged as progressive")
	}
}
