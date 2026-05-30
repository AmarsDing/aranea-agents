package cron

import (
	"encoding/json"
	"testing"
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
