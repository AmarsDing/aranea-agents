package tool

import (
	"strings"
	"testing"
)

func TestMergeToolConfigMaps(t *testing.T) {
	tests := []struct {
		name        string
		baseJSON    string
		defaultJSON string
		want        map[string]any
	}{
		{
			name:        "both empty",
			baseJSON:    "",
			defaultJSON: "",
			want:        map[string]any{},
		},
		{
			name:        "base empty default has data",
			baseJSON:    "",
			defaultJSON: `{"timeout": 30}`,
			want:        map[string]any{"timeout": float64(30)},
		},
		{
			name:        "default empty base has data",
			baseJSON:    `{"api_key": "abc"}`,
			defaultJSON: "",
			want:        map[string]any{"api_key": "abc"},
		},
		{
			name:        "both with data",
			baseJSON:    `{"api_key": "abc"}`,
			defaultJSON: `{"timeout": 30}`,
			want:        map[string]any{"api_key": "abc", "timeout": float64(30)},
		},
		{
			name:        "overlapping keys default wins",
			baseJSON:    `{"api_key": "base", "timeout": 10}`,
			defaultJSON: `{"api_key": "default"}`,
			want:        map[string]any{"api_key": "default", "timeout": float64(10)},
		},
		{
			name:        "invalid base JSON",
			baseJSON:    `{invalid}`,
			defaultJSON: `{"api_key": "default"}`,
			want:        map[string]any{"api_key": "default"},
		},
		{
			name:        "invalid default JSON",
			baseJSON:    `{"api_key": "base"}`,
			defaultJSON: `{invalid}`,
			want:        map[string]any{"api_key": "base"},
		},
		{
			name:        "both invalid JSON",
			baseJSON:    `{invalid}`,
			defaultJSON: `{also-invalid}`,
			want:        map[string]any{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MergeToolConfigMaps(tt.baseJSON, tt.defaultJSON)
			if len(got) != len(tt.want) {
				t.Fatalf("expected %d keys, got %d (got=%v)", len(tt.want), len(got), got)
			}
			for k, v := range tt.want {
				if got[k] != v {
					t.Errorf("key %q: expected %v, got %v", k, v, got[k])
				}
			}
		})
	}
}

func TestMergeJSONMapInto(t *testing.T) {
	tests := []struct {
		name string
		dst  map[string]any
		raw  string
		want map[string]any
	}{
		{
			name: "merge into empty",
			dst:  map[string]any{},
			raw:  `{"api_key": "abc"}`,
			want: map[string]any{"api_key": "abc"},
		},
		{
			name: "merge into existing",
			dst:  map[string]any{"existing": "val"},
			raw:  `{"api_key": "abc"}`,
			want: map[string]any{"existing": "val", "api_key": "abc"},
		},
		{
			name: "overlapping keys overwrite",
			dst:  map[string]any{"api_key": "old"},
			raw:  `{"api_key": "new"}`,
			want: map[string]any{"api_key": "new"},
		},
		{
			name: "invalid JSON no change",
			dst:  map[string]any{"existing": "val"},
			raw:  `{invalid}`,
			want: map[string]any{"existing": "val"},
		},
		{
			name: "empty string no change",
			dst:  map[string]any{"existing": "val"},
			raw:  "",
			want: map[string]any{"existing": "val"},
		},
		{
			name: "empty object no change",
			dst:  map[string]any{"existing": "val"},
			raw:  "{}",
			want: map[string]any{"existing": "val"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			MergeJSONMapInto(tt.dst, tt.raw)
			if len(tt.dst) != len(tt.want) {
				t.Fatalf("expected %d keys, got %d (got=%v)", len(tt.want), len(tt.dst), tt.dst)
			}
			for k, v := range tt.want {
				if tt.dst[k] != v {
					t.Errorf("key %q: expected %v, got %v", k, v, tt.dst[k])
				}
			}
		})
	}
}

func TestPropagateDenyAliases(t *testing.T) {
	tests := []struct {
		name string
		m    map[string]bool
		want map[string]bool
	}{
		{
			name: "deny alias propagates to canonical",
			m:    map[string]bool{"shell": true},
			want: map[string]bool{"shell": true, "shell_exec": true, "exec_command": true},
		},
		{
			name: "deny canonical propagates to alias",
			m:    map[string]bool{"shell_exec": true},
			want: map[string]bool{"shell_exec": true, "shell": true, "exec_command": true},
		},
		{
			name: "deny web_search alias propagates to web_research",
			m:    map[string]bool{"web_search": true},
			want: map[string]bool{"web_search": true, "web_research": true},
		},
		{
			name: "deny web_research canonical propagates to web_search",
			m:    map[string]bool{"web_research": true},
			want: map[string]bool{"web_research": true, "web_search": true},
		},
		{
			name: "deny all does not propagate to all tools",
			m:    map[string]bool{"all": true},
			want: map[string]bool{"all": true},
		},
		{
			name: "empty deny list",
			m:    map[string]bool{},
			want: map[string]bool{},
		},
		{
			name: "multiple denies propagate bidirectionally",
			m:    map[string]bool{"shell": true, "email": true},
			want: map[string]bool{"shell": true, "shell_exec": true, "exec_command": true, "email": true, "send_email": true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			PropagateDenyAliases(tt.m)
			if len(tt.m) != len(tt.want) {
				t.Fatalf("expected %d keys, got %d (got=%v)", len(tt.want), len(tt.m), tt.m)
			}
			for k, v := range tt.want {
				if tt.m[k] != v {
					t.Errorf("key %q: expected %v, got %v", k, v, tt.m[k])
				}
			}
		})
	}
}

func TestSanitizeToolInvocationWrite(t *testing.T) {
	longStr := strings.Repeat("a", 3000)

	tests := []struct {
		name  string
		in    *ToolInvocationWrite
		check func(t *testing.T, in *ToolInvocationWrite)
	}{
		{
			name:  "nil input no panic",
			in:    nil,
			check: func(t *testing.T, in *ToolInvocationWrite) {},
		},
		{
			name: "truncate long input_preview",
			in:   &ToolInvocationWrite{InputPreview: longStr},
			check: func(t *testing.T, in *ToolInvocationWrite) {
				if len(in.InputPreview) > toolPreviewMaxLen {
					t.Errorf("InputPreview not truncated: len=%d", len(in.InputPreview))
				}
			},
		},
		{
			name: "truncate long output_preview",
			in:   &ToolInvocationWrite{OutputPreview: longStr},
			check: func(t *testing.T, in *ToolInvocationWrite) {
				if len(in.OutputPreview) > toolPreviewMaxLen {
					t.Errorf("OutputPreview not truncated: len=%d", len(in.OutputPreview))
				}
			},
		},
		{
			name: "truncate long error_message",
			in:   &ToolInvocationWrite{ErrorMessage: longStr},
			check: func(t *testing.T, in *ToolInvocationWrite) {
				if len(in.ErrorMessage) > 500 {
					t.Errorf("ErrorMessage not truncated: len=%d", len(in.ErrorMessage))
				}
			},
		},
		{
			name: "empty fields preserved",
			in:   &ToolInvocationWrite{ToolKey: "test", InputPreview: "", OutputPreview: "", ErrorMessage: ""},
			check: func(t *testing.T, in *ToolInvocationWrite) {
				if in.InputPreview != "" {
					t.Errorf("InputPreview should be empty, got %q", in.InputPreview)
				}
				if in.OutputPreview != "" {
					t.Errorf("OutputPreview should be empty, got %q", in.OutputPreview)
				}
				if in.ErrorMessage != "" {
					t.Errorf("ErrorMessage should be empty, got %q", in.ErrorMessage)
				}
			},
		},
		{
			name: "other fields untouched",
			in:   &ToolInvocationWrite{ToolKey: "shell_exec", Status: "error", DurationMS: 42},
			check: func(t *testing.T, in *ToolInvocationWrite) {
				if in.ToolKey != "shell_exec" {
					t.Errorf("ToolKey changed: got %q", in.ToolKey)
				}
				if in.Status != "error" {
					t.Errorf("Status changed: got %q", in.Status)
				}
				if in.DurationMS != 42 {
					t.Errorf("DurationMS changed: got %d", in.DurationMS)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			SanitizeToolInvocationWrite(tt.in)
			tt.check(t, tt.in)
		})
	}
}

func TestHasOpenAPIMetadata(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want bool
	}{
		{
			name: "with openapi_spec_url",
			raw:  `{"openapi_spec_url": "http://example.com/spec"}`,
			want: true,
		},
		{
			name: "with openapi_spec_data",
			raw:  `{"openapi_spec_data": "some spec content"}`,
			want: true,
		},
		{
			name: "with spec_url",
			raw:  `{"spec_url": "http://example.com/spec"}`,
			want: true,
		},
		{
			name: "with spec_data",
			raw:  `{"spec_data": "some spec content"}`,
			want: true,
		},
		{
			name: "with neither",
			raw:  `{"other_key": "value"}`,
			want: false,
		},
		{
			name: "with both spec_url and spec_data",
			raw:  `{"spec_url": "http://example.com/spec", "spec_data": "content"}`,
			want: true,
		},
		{
			name: "empty string",
			raw:  "",
			want: false,
		},
		{
			name: "empty object",
			raw:  "{}",
			want: false,
		},
		{
			name: "invalid JSON",
			raw:  "{invalid}",
			want: false,
		},
		{
			name: "spec_url empty value",
			raw:  `{"spec_url": ""}`,
			want: false,
		},
		{
			name: "spec_url whitespace only",
			raw:  `{"spec_url": "   "}`,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := HasOpenAPIMetadata(tt.raw)
			if got != tt.want {
				t.Errorf("HasOpenAPIMetadata(%q) = %v, want %v", tt.raw, got, tt.want)
			}
		})
	}
}

func TestToolConfigReady(t *testing.T) {

	tests := []struct {
		name     string
		tool     Tool
		platform *WebResearchSetting
		want     bool
	}{
		{
			name: "google_search ready",
			tool: Tool{
				Key:               "google_search",
				ConfigJSON:        `{"api_key": "test", "cx": "my-cx"}`,
				DefaultConfigJSON: "",
			},
			platform: nil,
			want:     true,
		},
		{
			name: "google_search missing api_key",
			tool: Tool{
				Key:               "google_search",
				ConfigJSON:        `{"cx": "my-cx"}`,
				DefaultConfigJSON: "",
			},
			platform: nil,
			want:     false,
		},
		{
			name: "google_search missing cx",
			tool: Tool{
				Key:               "google_search",
				ConfigJSON:        `{"api_key": "test"}`,
				DefaultConfigJSON: "",
			},
			platform: nil,
			want:     false,
		},
		{
			name: "google_search with google_api_key alias",
			tool: Tool{
				Key:               "google_search",
				ConfigJSON:        `{"google_api_key": "test", "search_engine_id": "my-cx"}`,
				DefaultConfigJSON: "",
			},
			platform: nil,
			want:     true,
		},
		{
			name: "gemini_web_fetch ready",
			tool: Tool{
				Key:               "gemini_web_fetch",
				ConfigJSON:        `{"model": "gemini-pro"}`,
				DefaultConfigJSON: "",
			},
			platform: nil,
			want:     true,
		},
		{
			name: "gemini_web_fetch missing model",
			tool: Tool{
				Key:               "gemini_web_fetch",
				ConfigJSON:        `{}`,
				DefaultConfigJSON: "",
			},
			platform: nil,
			want:     false,
		},
		{
			name: "gemini_web_fetch with gemini_model alias",
			tool: Tool{
				Key:               "gemini_web_fetch",
				ConfigJSON:        `{"gemini_model": "gemini-pro"}`,
				DefaultConfigJSON: "",
			},
			platform: nil,
			want:     true,
		},
		{
			name: "web_research ready via platform HasAPIKey",
			tool: Tool{
				Key:               "web_research",
				ConfigJSON:        "",
				DefaultConfigJSON: "",
			},
			platform: &WebResearchSetting{HasAPIKey: true},
			want:     true,
		},
		{
			name: "web_research ready via platform APIKey",
			tool: Tool{
				Key:               "web_research",
				ConfigJSON:        "",
				DefaultConfigJSON: "",
			},
			platform: &WebResearchSetting{APIKey: "sk-test"},
			want:     true,
		},
		{
			name: "web_research ready via config api_key",
			tool: Tool{
				Key:               "web_research",
				ConfigJSON:        `{"api_key": "sk-test"}`,
				DefaultConfigJSON: "",
			},
			platform: nil,
			want:     true,
		},
		{
			name: "web_research not ready",
			tool: Tool{
				Key:               "web_research",
				ConfigJSON:        "",
				DefaultConfigJSON: "",
			},
			platform: nil,
			want:     false,
		},
		{
			name: "default tool always ready",
			tool: Tool{
				Key:               "shell_exec",
				ConfigJSON:        "",
				DefaultConfigJSON: "",
			},
			platform: nil,
			want:     true,
		},
		{
			name: "empty config default tool ready",
			tool: Tool{
				Key:               "read_file",
				ConfigJSON:        "",
				DefaultConfigJSON: "",
			},
			platform: nil,
			want:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ToolConfigReady(tt.tool, tt.platform)
			if got != tt.want {
				t.Errorf("ToolConfigReady() = %v, want %v", got, tt.want)
			}
		})
	}
}
