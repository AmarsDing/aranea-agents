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
