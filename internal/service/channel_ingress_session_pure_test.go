package service

import (
	"testing"
)

func TestPlatformFromTitlePrefix(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"simple", "feishu", "feishu"},
		{"with_colon", "feishu:main", "feishu"},
		{"with_multiple_colons", "feishu:main:extra", "feishu"},
		{"whitespace_prefix", "  feishu:main", "feishu"},
		{"whitespace_around_colon", "feishu : main", "feishu"},
		{"empty", "", ""},
		{"whitespace_only", "   ", ""},
		{"single_word", "slack", "slack"},
		{"wechat_with_colon", "wechat:channel1", "wechat"},
		{"dingtalk_with_colon", "dingtalk:bot1", "dingtalk"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := platformFromTitlePrefix(tt.input)
			if got != tt.want {
				t.Errorf("platformFromTitlePrefix(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
