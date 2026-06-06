package service

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	kerrors "github.com/go-kratos/kratos/v2/errors"

	"aranea-agents/internal/biz/monitor"
	"aranea-agents/pkg/loggateway"
)

func containsSubstr(s, sub string) bool {
	return strings.Contains(s, sub)
}

func TestParseFlowLogTimeBounds(t *testing.T) {
	cases := []struct {
		name      string
		sinceRaw  string
		untilRaw  string
		wantSince time.Time
		wantUntil time.Time
		wantErr   bool
		errMsg string
	}{
		{
			name:      "both_empty",
			sinceRaw:  "",
			untilRaw:  "",
			wantSince: time.Time{},
			wantUntil: time.Time{},
		},
		{
			name:      "valid_since_and_until",
			sinceRaw:  "2025-01-01T00:00:00Z",
			untilRaw:  "2025-01-02T00:00:00Z",
			wantSince: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			wantUntil: time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC),
		},
		{
			name:      "valid_since_and_until_nano",
			sinceRaw:  "2025-01-01T00:00:00.123456789Z",
			untilRaw:  "2025-01-02T00:00:00.999999999Z",
			wantSince: time.Date(2025, 1, 1, 0, 0, 0, 123456789, time.UTC),
			wantUntil: time.Date(2025, 1, 2, 0, 0, 0, 999999999, time.UTC),
		},
		{
			name:      "only_since",
			sinceRaw:  "2025-03-15T10:30:00Z",
			untilRaw:  "",
			wantSince: time.Date(2025, 3, 15, 10, 30, 0, 0, time.UTC),
			wantUntil: time.Time{},
		},
		{
			name:      "only_until",
			sinceRaw:  "",
			untilRaw:  "2025-06-01T12:00:00Z",
			wantSince: time.Time{},
			wantUntil: time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC),
		},
		{
			name:      "whitespace_only",
			sinceRaw:  "   ",
			untilRaw:  "  ",
			wantSince: time.Time{},
			wantUntil: time.Time{},
		},
		{
			name:      "invalid_since",
			sinceRaw:  "not-a-date",
			untilRaw:  "",
			wantErr:   true,
			errMsg: "invalid since",
		},
		{
			name:      "invalid_until",
			sinceRaw:  "",
			untilRaw:  "bad-time",
			wantErr:   true,
			errMsg: "invalid until",
		},
		{
			name:      "until_before_since",
			sinceRaw:  "2025-06-01T00:00:00Z",
			untilRaw:  "2025-01-01T00:00:00Z",
			wantErr:   true,
			errMsg: "until must be after since",
		},
		{
			name:      "until_equals_since",
			sinceRaw:  "2025-03-01T00:00:00Z",
			untilRaw:  "2025-03-01T00:00:00Z",
			wantSince: time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC),
			wantUntil: time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			name:      "since_with_timezone_offset",
			sinceRaw:  "2025-01-01T08:00:00+08:00",
			untilRaw:  "",
			wantSince: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			since, until, err := monitor.ParseFlowLogTimeBounds(tc.sinceRaw, tc.untilRaw)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				ke := kerrors.FromError(err)
			if ke == nil {
				t.Fatalf("expected kerrors, got %T: %v", err, err)
			}
			if tc.errMsg != "" && !containsSubstr(ke.Message, tc.errMsg) {
				t.Fatalf("message = %q, want containing %q", ke.Message, tc.errMsg)
			}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !since.Equal(tc.wantSince) {
				t.Fatalf("since = %v, want %v", since, tc.wantSince)
			}
			if !until.Equal(tc.wantUntil) {
				t.Fatalf("until = %v, want %v", until, tc.wantUntil)
			}
		})
	}
}

func TestIsSensitiveKey(t *testing.T) {
	cases := []struct {
		name string
		key  string
		want bool
	}{
		{"api_key", "api_key", true},
		{"apikey", "apikey", true},
		{"token", "token", true},
		{"secret", "secret", true},
		{"password", "password", true},
		{"authorization", "authorization", true},
		{"cookie", "cookie", true},
		{"normal_key", "name", false},
		{"empty", "", false},
		{"prefix_api_key", "x_api_key", true},
		{"suffix_token", "access_token", true},
		{"contains_secret", "client_secret_id", true},
		{"case_insensitive_upper", "API_KEY", true},
		{"case_insensitive_mixed", "Api_Key", true},
		{"case_insensitive_token", "TOKEN", true},
		{"case_insensitive_password", "Password", true},
		{"case_insensitive_authorization", "Authorization", true},
		{"case_insensitive_cookie", "COOKIE", true},
		{"normal_with_spaces", "  api_key  ", true},
		{"non_sensitive", "display_name", false},
		{"non_sensitive_model", "model", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := monitor.IsSensitiveKey(tc.key)
			if got != tc.want {
				t.Fatalf("IsSensitiveKey(%q) = %v, want %v", tc.key, got, tc.want)
			}
		})
	}
}

func TestSanitizeJSONValue(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want any
	}{
		{"string_value", "hello", "hello"},
		{"number_value", float64(42), float64(42)},
		{"bool_value", true, true},
		{"nil_value", nil, nil},
		{"map_with_sensitive_key", map[string]any{"api_key": "sk-123", "name": "test"}, map[string]any{"api_key": "******", "name": "test"}},
		{"nested_map", map[string]any{"config": map[string]any{"token": "abc", "port": float64(8080)}}, map[string]any{"config": map[string]any{"token": "******", "port": float64(8080)}}},
		{"array_value", []any{"a", float64(1)}, []any{"a", float64(1)}},
		{"array_with_map", []any{map[string]any{"password": "pw"}}, []any{map[string]any{"password": "******"}}},
		{"empty_map", map[string]any{}, map[string]any{}},
		{"empty_array", []any{}, []any{}},
		{"deeply_nested", map[string]any{
			"outer": map[string]any{
				"inner": map[string]any{
					"secret": "hidden",
				},
			},
		}, map[string]any{
			"outer": map[string]any{
				"inner": map[string]any{
					"secret": "******",
				},
			},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := monitor.SanitizeJSONValue(tc.in)
			gotJSON, _ := json.Marshal(got)
			wantJSON, _ := json.Marshal(tc.want)
			if string(gotJSON) != string(wantJSON) {
				t.Fatalf("got %s, want %s", gotJSON, wantJSON)
			}
		})
	}
}

func TestSanitizeJSONString(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "valid_json_with_sensitive_key",
			raw:  `{"api_key":"sk-123","name":"test"}`,
			want: `{"api_key":"******","name":"test"}`,
		},
		{
			name: "no_sensitive_keys",
			raw:  `{"name":"test","port":8080}`,
			want: `{"name":"test","port":8080}`,
		},
		{
			name: "invalid_json",
			raw:  `{not json}`,
			want: `{not json}`,
		},
		{
			name: "empty_string",
			raw:  "",
			want: "",
		},
		{
			name: "whitespace_only",
			raw:  "   ",
			want: "   ",
		},
		{
			name: "nested_sensitive",
			raw:  `{"config":{"token":"abc","port":8080}}`,
			want: `{"config":{"port":8080,"token":"******"}}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := monitor.SanitizeJSONString(tc.raw, loggateway.Global())
			if tc.name == "invalid_json" || tc.name == "empty_string" || tc.name == "whitespace_only" {
				if got != tc.want {
					t.Fatalf("got %q, want %q", got, tc.want)
				}
				return
			}
			var gotMap, wantMap map[string]any
			if err := json.Unmarshal([]byte(got), &gotMap); err != nil {
				t.Fatalf("got is not valid JSON: %q", got)
			}
			if err := json.Unmarshal([]byte(tc.want), &wantMap); err != nil {
				t.Fatalf("want is not valid JSON: %q", tc.want)
			}
			gotJSON, _ := json.Marshal(gotMap)
			wantJSON, _ := json.Marshal(wantMap)
			if string(gotJSON) != string(wantJSON) {
				t.Fatalf("got %s, want %s", gotJSON, wantJSON)
			}
		})
	}
}

func TestParseJSONMap(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want map[string]any
	}{
		{
			name: "valid_json",
			raw:  `{"name":"test","port":8080}`,
			want: map[string]any{"name": "test", "port": float64(8080)},
		},
		{
			name: "invalid_json",
			raw:  `{not json}`,
			want: map[string]any{},
		},
		{
			name: "empty_string",
			raw:  "",
			want: map[string]any{},
		},
		{
			name: "whitespace_only",
			raw:  "   ",
			want: map[string]any{},
		},
		{
			name: "nested_sensitive",
			raw:  `{"api_key":"sk-123","config":{"token":"abc"}}`,
			want: map[string]any{"api_key": "******", "config": map[string]any{"token": "******"}},
		},
		{
			name: "empty_object",
			raw:  `{}`,
			want: map[string]any{},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := monitor.ParseJSONMap(tc.raw, loggateway.NewNoop())
			gotJSON, _ := json.Marshal(got)
			wantJSON, _ := json.Marshal(tc.want)
			if string(gotJSON) != string(wantJSON) {
				t.Fatalf("got %s, want %s", gotJSON, wantJSON)
			}
		})
	}
}
