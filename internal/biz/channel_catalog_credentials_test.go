package biz_test

import (
	"testing"

	"aranea-agents/internal/biz"
)

func TestCredentialSchemaFor(t *testing.T) {
	cases := []struct {
		name        string
		channelType string
		wantProps   bool
		wantType    string
	}{
		{"telegram has properties", "telegram", true, "object"},
		{"feishu has properties", "feishu", true, "object"},
		{"slack has properties", "slack", true, "object"},
		{"discord has properties", "discord", true, "object"},
		{"dingtalk has properties", "dingtalk", true, "object"},
		{"wechat has properties", "wechat", true, "object"},
		{"wecom has properties", "wecom", true, "object"},
		{"wecom-app has properties", "wecom-app", true, "object"},
		{"qq has properties", "qq", true, "object"},
		{"personal_qq has properties", "personal_qq", true, "object"},
		{"line has properties", "line", true, "object"},
		{"mattermost has properties", "mattermost", true, "object"},
		{"teams has properties", "teams", true, "object"},
		{"unknown no properties", "unknown", false, "object"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			schema := biz.CredentialSchemaFor(tc.channelType)
			if schema["type"] != tc.wantType {
				t.Fatalf("type = %v, want %q", schema["type"], tc.wantType)
			}
			_, hasProps := schema["properties"]
			if hasProps != tc.wantProps {
				t.Fatalf("has properties = %v, want %v", hasProps, tc.wantProps)
			}
		})
	}
}

func TestCredentialProperties(t *testing.T) {
	cases := []struct {
		name        string
		channelType string
		wantFields  []string
	}{
		{"telegram", "telegram", []string{"bot_token"}},
		{"feishu", "feishu", []string{"app_secret"}},
		{"slack", "slack", []string{"bot_token", "app_token", "signing_secret"}},
		{"dingtalk", "dingtalk", []string{"client_secret", "secret"}},
		{"wechat", "wechat", []string{"app_secret", "token", "encoding_aes_key"}},
		{"wecom", "wecom", []string{"token", "encoding_aes_key", "corp_secret"}},
		{"qq", "qq", []string{"app_secret"}},
		{"personal_qq", "personal_qq", []string{"receive_token", "send_token"}},
		{"line", "line", []string{"channel_secret", "channel_token"}},
		{"mattermost", "mattermost", []string{"server_url", "bot_token"}},
		{"teams", "teams", []string{"app_id", "app_secret"}},
		{"unknown", "unknown", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			props := biz.CredentialProperties(tc.channelType)
			if tc.wantFields == nil {
				if props != nil {
					t.Fatalf("expected nil, got %v", props)
				}
				return
			}
			for _, field := range tc.wantFields {
				if _, ok := props[field]; !ok {
					t.Fatalf("missing field %q in properties", field)
				}
			}
			if len(props) != len(tc.wantFields) {
				t.Fatalf("got %d fields, want %d", len(props), len(tc.wantFields))
			}
		})
	}
}

func TestPropField(t *testing.T) {
	cases := []struct {
		name     string
		title    string
		format   string
		required bool
		want     map[string]any
	}{
		{
			name:   "with format and required",
			title:  "test_title",
			format: "password",
			required: true,
			want: map[string]any{
				"type":       "string",
				"title":      "test_title",
				"format":     "password",
				"x-required": true,
			},
		},
		{
			name:   "without format",
			title:  "test_title",
			format: "",
			required: true,
			want: map[string]any{
				"type":       "string",
				"title":      "test_title",
				"x-required": true,
			},
		},
		{
			name:   "not required",
			title:  "test_title",
			format: "password",
			required: false,
			want: map[string]any{
				"type":   "string",
				"title":  "test_title",
				"format": "password",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := biz.PropField(tc.title, tc.format, tc.required)
			if got["type"] != tc.want["type"] {
				t.Fatalf("type = %v, want %v", got["type"], tc.want["type"])
			}
			if got["title"] != tc.want["title"] {
				t.Fatalf("title = %v, want %v", got["title"], tc.want["title"])
			}
			if tc.format == "" {
				if _, hasFormat := got["format"]; hasFormat {
					t.Fatalf("unexpected format key present")
				}
			} else {
				if got["format"] != tc.want["format"] {
					t.Fatalf("format = %v, want %v", got["format"], tc.want["format"])
				}
			}
			if tc.required {
				if got["x-required"] != tc.want["x-required"] {
					t.Fatalf("x-required = %v, want %v", got["x-required"], tc.want["x-required"])
				}
			} else {
				if _, hasReq := got["x-required"]; hasReq {
					t.Fatalf("unexpected x-required key present")
				}
			}
		})
	}
}
