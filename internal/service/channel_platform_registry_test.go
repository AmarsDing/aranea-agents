package service

import "testing"

func TestStreamPlatformSupported(t *testing.T) {
	cases := []struct {
		platform string
		want     bool
	}{
		{"feishu", true},
		{"slack", true},
		{"telegram", true},
		{"line", true},
		{"mattermost", true},
		{"dingtalk", false},
		{"wecom", false},
		{"discord", false},
		{"wechat", false},
		{"qq", false},
		{"personal_qq", false},
		{"teams", false},
		{"unknown", false},
		{"", false},
		{"Feishu", true},
		{"  telegram  ", true},
	}
	for _, tc := range cases {
		got := streamPlatformSupported(tc.platform)
		if got != tc.want {
			t.Errorf("streamPlatformSupported(%q) = %v, want %v", tc.platform, got, tc.want)
		}
	}
}
