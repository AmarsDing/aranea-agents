package service

import (
	"testing"

	v1 "aranea-agents/api/kratos/llm_provider_model/v1"
	"aranea-agents/internal/biz"
)

func TestPatchFromProto(t *testing.T) {
	cases := []struct {
		name string
		pb   *v1.ProviderModel
		want biz.ProviderModel
	}{
		{
			name: "nil_input",
			pb:   nil,
			want: biz.ProviderModel{},
		},
		{
			name: "full_fields",
			pb: &v1.ProviderModel{
				Key:          "gpt-4o",
				Name:         "GPT-4o",
				Description:  "desc",
				Status:       "active",
				Enabled:      true,
				SortOrder:    10,
				Provider:     "openai",
				Model:        "gpt-4o",
				ConfigJson:   `{"api_key":"sk-123"}`,
				MetadataJson: `{"tier":"premium"}`,
			},
			want: biz.ProviderModel{
				Key:          "gpt-4o",
				Name:         "GPT-4o",
				Description:  "desc",
				Status:       "active",
				Enabled:      true,
				SortOrder:    10,
				Provider:     "openai",
				Model:        "gpt-4o",
				ConfigJSON:   `{"api_key":"sk-123"}`,
				MetadataJSON: `{"tier":"premium"}`,
			},
		},
		{
			name: "with_capabilities",
			pb: &v1.ProviderModel{
				Key:     "claude",
				Name:    "Claude",
				Enabled: true,
				Capabilities: &v1.ModelCapabilities{
					Text:     true,
					Vision:   true,
					ToolCall: true,
				},
			},
			want: biz.ProviderModel{
				Key:                  "claude",
				Name:                 "Claude",
				Enabled:              true,
				Capabilities:         biz.ModelCapabilities{Text: true, Vision: true, ToolCall: true},
				CapabilitiesExplicit: true,
			},
		},
		{
			name: "nil_capabilities",
			pb: &v1.ProviderModel{
				Key:  "model-1",
				Name: "Model One",
			},
			want: biz.ProviderModel{
				Key:  "model-1",
				Name: "Model One",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := patchFromProto(tc.pb)
			if got.Key != tc.want.Key || got.Name != tc.want.Name || got.Description != tc.want.Description ||
				got.Status != tc.want.Status || got.Enabled != tc.want.Enabled || got.SortOrder != tc.want.SortOrder ||
				got.Provider != tc.want.Provider || got.Model != tc.want.Model ||
				got.ConfigJSON != tc.want.ConfigJSON || got.MetadataJSON != tc.want.MetadataJSON ||
				got.CapabilitiesExplicit != tc.want.CapabilitiesExplicit {
				t.Fatalf("got=%+v, want=%+v", got, tc.want)
			}
			if got.Capabilities != tc.want.Capabilities {
				t.Fatalf("capabilities got=%+v, want=%+v", got.Capabilities, tc.want.Capabilities)
			}
		})
	}
}

func TestCapabilitiesFromProto(t *testing.T) {
	cases := []struct {
		name string
		caps *v1.ModelCapabilities
		want biz.ModelCapabilities
	}{
		{
			name: "nil_input",
			caps: nil,
			want: biz.ModelCapabilities{},
		},
		{
			name: "all_fields",
			caps: &v1.ModelCapabilities{
				Text:     true,
				Vision:   true,
				Audio:    true,
				File:     true,
				ToolCall: true,
				Cache:    true,
				Thinking: true,
				TextOnly: true,
			},
			want: biz.ModelCapabilities{
				Text:     true,
				Vision:   true,
				Audio:    true,
				File:     true,
				ToolCall: true,
				Cache:    true,
				Thinking: true,
				TextOnly: true,
			},
		},
		{
			name: "partial_fields",
			caps: &v1.ModelCapabilities{
				Text:   true,
				Vision: true,
			},
			want: biz.ModelCapabilities{
				Text:   true,
				Vision: true,
			},
		},
		{
			name: "all_false",
			caps: &v1.ModelCapabilities{},
			want: biz.ModelCapabilities{},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := capabilitiesFromProto(tc.caps)
			if got != tc.want {
				t.Fatalf("got=%+v, want=%+v", got, tc.want)
			}
		})
	}
}

func TestHasExplicitBizCapabilities(t *testing.T) {
	cases := []struct {
		name string
		c    biz.ModelCapabilities
		want bool
	}{
		{"all_false", biz.ModelCapabilities{}, false},
		{"text_true", biz.ModelCapabilities{Text: true}, true},
		{"vision_true", biz.ModelCapabilities{Vision: true}, true},
		{"audio_true", biz.ModelCapabilities{Audio: true}, true},
		{"file_true", biz.ModelCapabilities{File: true}, true},
		{"tool_call_true", biz.ModelCapabilities{ToolCall: true}, true},
		{"cache_true", biz.ModelCapabilities{Cache: true}, true},
		{"thinking_true", biz.ModelCapabilities{Thinking: true}, true},
		{"text_only_true", biz.ModelCapabilities{TextOnly: true}, true},
		{"all_true", biz.ModelCapabilities{Text: true, Vision: true, Audio: true, File: true, ToolCall: true, Cache: true, Thinking: true, TextOnly: true}, true},
		{"zero_value", biz.ModelCapabilities{}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := hasExplicitBizCapabilities(tc.c)
			if got != tc.want {
				t.Fatalf("got=%v, want=%v", got, tc.want)
			}
		})
	}
}
