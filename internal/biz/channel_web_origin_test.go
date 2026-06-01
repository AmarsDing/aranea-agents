package biz_test

import (
	"testing"

	"aranea-agents/internal/biz"
)

func TestResolveChannelWebOrigin(t *testing.T) {
	cases := []struct {
		name         string
		metadataJSON string
		want         string
	}{
		{"empty string", "", ""},
		{"whitespace", "  ", ""},
		{"invalid json", "not json", ""},
		{"web_app_origin", `{"web_app_origin":"https://app.example.com"}`, "https://app.example.com"},
		{"web_app_origin with trailing slash", `{"web_app_origin":"https://app.example.com/"}`, "https://app.example.com"},
		{"falls back to public_webhook_origin", `{"public_webhook_origin":"https://hook.example.com"}`, "https://hook.example.com"},
		{"web_app_origin preferred", `{"web_app_origin":"https://app.example.com","public_webhook_origin":"https://hook.example.com"}`, "https://app.example.com"},
		{"both empty", `{"web_app_origin":"","public_webhook_origin":""}`, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := biz.ResolveChannelWebOrigin(tc.metadataJSON)
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestNormalizeOrigin(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{"normal", "https://example.com", "https://example.com"},
		{"trailing slash", "https://example.com/", "https://example.com"},
		{"multiple trailing slashes", "https://example.com///", "https://example.com"},
		{"with spaces", "  https://example.com/  ", "https://example.com"},
		{"empty", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := biz.NormalizeOrigin(tc.raw)
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}
