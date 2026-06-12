package biz

import (
	"strings"
	"testing"
)

func TestRequireNonEmpty(t *testing.T) {
	cases := []struct {
		val    string
		domain string
		field  string
		wantOK bool
	}{
		{"hello", "AGENT", "name", true},
		{"  hello  ", "AGENT", "name", true},
		{"", "AGENT", "name", false},
		{"   ", "AGENT", "name", false},
	}
	for _, tc := range cases {
		val, err := requireNonEmpty(tc.val, tc.domain, tc.field)
		if tc.wantOK {
			if err != nil {
				t.Errorf("requireNonEmpty(%q) unexpected error: %v", tc.val, err)
			}
			if strings.TrimSpace(val) != strings.TrimSpace(tc.val) {
				t.Errorf("requireNonEmpty(%q) val=%q", tc.val, val)
			}
		} else {
			if err == nil {
				t.Errorf("requireNonEmpty(%q) expected error", tc.val)
			}
		}
	}
}

func TestInboundIdempotencyKey(t *testing.T) {
	cases := []struct {
		platform   string
		messageKey string
		want       string
	}{
		{"feishu", "om_123", "om_123"},
		{"feishu", "  om_456  ", "om_456"},
		{"feishu", "", ""},
	}
	for _, tc := range cases {
		got := InboundIdempotencyKey(tc.platform, tc.messageKey)
		if got != tc.want {
			t.Errorf("InboundIdempotencyKey(%q, %q) = %q, want %q",
				tc.platform, tc.messageKey, got, tc.want)
		}
	}
}

func TestInboundTextPreview(t *testing.T) {
	cases := []struct {
		text string
		want string
	}{
		{"short message", "short message"},
		{"  trimmed  ", "trimmed"},
		{"", ""},
		{strings.Repeat("a", 130), strings.Repeat("a", 120) + "…"},
		{strings.Repeat("b", 120), strings.Repeat("b", 120)},
	}
	for _, tc := range cases {
		got := inboundTextPreview(tc.text)
		if got != tc.want {
			t.Errorf("inboundTextPreview(%q) = %q (len=%d), want %q (len=%d)",
				tc.text[:minInt(len(tc.text), 30)], got, len(got), tc.want[:minInt(len(tc.want), 30)], len(tc.want))
		}
	}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
