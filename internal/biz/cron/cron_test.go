package cron

import (
	"encoding/json"
	"testing"

	"aranea-agents/pkg/apierror"
)

func TestStrPtr(t *testing.T) {
	v := "hello"
	p := StrPtr(v)
	if p == nil {
		t.Fatal("StrPtr returned nil")
	}
	if *p != v {
		t.Errorf("*StrPtr(%q) = %q, want %q", v, *p, v)
	}
}

func TestBoolPtr(t *testing.T) {
	v := true
	p := BoolPtr(v)
	if p == nil {
		t.Fatal("BoolPtr returned nil")
	}
	if *p != v {
		t.Errorf("*BoolPtr(%v) = %v, want %v", v, *p, v)
	}
}

func TestIntPtr(t *testing.T) {
	v := 42
	p := IntPtr(v)
	if p == nil {
		t.Fatal("IntPtr returned nil")
	}
	if *p != v {
		t.Errorf("*IntPtr(%d) = %d, want %d", v, *p, v)
	}
}

func TestResetFailureMetadata(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		wantErr bool
		check   func(t *testing.T, result string)
	}{
		{
			"empty string produces defaults",
			"",
			false,
			func(t *testing.T, result string) {
				var m map[string]any
				if err := json.Unmarshal([]byte(result), &m); err != nil {
					t.Fatalf("unmarshal: %v", err)
				}
				if fc, ok := m["failure_count"].(float64); !ok || fc != 0 {
					t.Errorf("failure_count = %v, want 0", m["failure_count"])
				}
				if le, ok := m["last_error"].(string); !ok || le != "" {
					t.Errorf("last_error = %v, want empty string", m["last_error"])
				}
				if rf, ok := m["recent_failures"].([]any); !ok || len(rf) != 0 {
					t.Errorf("recent_failures = %v, want empty array", m["recent_failures"])
				}
			},
		},
		{
			"whitespace string produces defaults",
			"   ",
			false,
			func(t *testing.T, result string) {
				var m map[string]any
				if err := json.Unmarshal([]byte(result), &m); err != nil {
					t.Fatalf("unmarshal: %v", err)
				}
				if fc, ok := m["failure_count"].(float64); !ok || fc != 0 {
					t.Errorf("failure_count = %v, want 0", m["failure_count"])
				}
			},
		},
		{
			"clears existing failure fields",
			`{"failure_count":5,"last_error":"timeout","recent_failures":["err1","err2"],"other":"kept"}`,
			false,
			func(t *testing.T, result string) {
				var m map[string]any
				if err := json.Unmarshal([]byte(result), &m); err != nil {
					t.Fatalf("unmarshal: %v", err)
				}
				if fc, ok := m["failure_count"].(float64); !ok || fc != 0 {
					t.Errorf("failure_count = %v, want 0", m["failure_count"])
				}
				if le, ok := m["last_error"].(string); !ok || le != "" {
					t.Errorf("last_error = %v, want empty string", m["last_error"])
				}
				if rf, ok := m["recent_failures"].([]any); !ok || len(rf) != 0 {
					t.Errorf("recent_failures = %v, want empty array", m["recent_failures"])
				}
				if o, ok := m["other"].(string); !ok || o != "kept" {
					t.Errorf("other = %v, want %q", m["other"], "kept")
				}
			},
		},
		{
			"invalid json returns error",
			"not-json",
			true,
			nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResetFailureMetadata(tt.raw)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.check != nil {
				tt.check(t, got)
			}
		})
	}
}

func TestValidateTaskConfig(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"empty string passes", "", false},
		{"whitespace string passes", "   ", false},
		{"valid json object passes", `{"target_type":"agent"}`, false},
		{"valid json with multiple fields passes", `{"target_type":"team","cron_expression":"0 * * * *"}`, false},
		{"valid json with model_registry_sync passes", `{"target_type":"model_registry_sync"}`, false},
		{"invalid json rejected", "not-json", true},
		{"invalid json with trailing comma rejected", `{"target_type":"agent",}`, true},
		{"target_type not string rejected", `{"target_type":123}`, true},
		{"target_type invalid value rejected", `{"target_type":"invalid"}`, true},
		{"cron_expression not string rejected", `{"cron_expression":123}`, true},
		{"cron_expression empty string rejected", `{"cron_expression":""}`, true},
		{"cron_expression whitespace only rejected", `{"cron_expression":"  "}`, true},
		{"valid cron_expression passes", `{"cron_expression":"*/5 * * * *"}`, false},
		{"json without target_type or cron_expression passes", `{"other":"field"}`, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateTaskConfig(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidateTaskConfig(%q) err = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if err != nil && !isAPIErrorCode(err, apierror.CodeBadRequest) {
				t.Errorf("expected BadRequest error, got %v", err)
			}
		})
	}
}

func isAPIErrorCode(err error, code apierror.Code) bool {
	ae, ok := apierror.From(err)
	return ok && ae.Code == code
}
