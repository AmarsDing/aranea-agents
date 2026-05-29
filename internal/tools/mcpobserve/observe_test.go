package mcpobserve

import (
	"testing"
	"time"
)

func TestIsRecentReconnect(t *testing.T) {
	tests := []struct {
		name            string
		lastReconnectAt string
		want            bool
	}{
		{
			name:            "empty_string_returns_false",
			lastReconnectAt: "",
			want:            false,
		},
		{
			name:            "whitespace_only_returns_false",
			lastReconnectAt: "   ",
			want:            false,
		},
		{
			name:            "invalid_time_returns_false",
			lastReconnectAt: "not-a-time",
			want:            false,
		},
		{
			name:            "future_time_returns_false",
			lastReconnectAt: time.Now().Add(1 * time.Hour).Format(time.RFC3339),
			want:            false,
		},
		{
			name:            "recent_time_within_window_returns_true",
			lastReconnectAt: time.Now().Add(-1 * time.Minute).Format(time.RFC3339),
			want:            true,
		},
		{
			name:            "time_exactly_at_window_edge_returns_false",
			lastReconnectAt: time.Now().Add(-RecentReconnectWindow).Format(time.RFC3339),
			want:            false,
		},
		{
			name:            "time_just_before_window_returns_false",
			lastReconnectAt: time.Now().Add(-RecentReconnectWindow - time.Second).Format(time.RFC3339),
			want:            false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsRecentReconnect(tt.lastReconnectAt)
			if got != tt.want {
				t.Errorf("IsRecentReconnect(%q) = %v, want %v", tt.lastReconnectAt, got, tt.want)
			}
		})
	}
}

func TestEffectiveSessionReconnectMax(t *testing.T) {
	tests := []struct {
		name        string
		transport   string
		configured  int
		want        int
	}{
		{
			name:       "configured_positive_returns_configured",
			transport:  "sse",
			configured: 5,
			want:       5,
		},
		{
			name:       "configured_positive_overrides_stdio",
			transport:  "stdio",
			configured: 7,
			want:       7,
		},
		{
			name:       "configured_zero_sse_returns_default",
			transport:  "sse",
			configured: 0,
			want:       3,
		},
		{
			name:       "configured_zero_streamable_returns_default",
			transport:  "streamable",
			configured: 0,
			want:       3,
		},
		{
			name:       "configured_zero_streamable_http_alias_returns_default",
			transport:  "streamable_http",
			configured: 0,
			want:       3,
		},
		{
			name:       "configured_zero_http_alias_returns_default",
			transport:  "http",
			configured: 0,
			want:       3,
		},
		{
			name:       "configured_zero_stdio_returns_zero",
			transport:  "stdio",
			configured: 0,
			want:       0,
		},
		{
			name:       "configured_negative_sse_returns_default",
			transport:  "sse",
			configured: -1,
			want:       3,
		},
		{
			name:       "configured_negative_stdio_returns_zero",
			transport:  "stdio",
			configured: -1,
			want:       0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EffectiveSessionReconnectMax(tt.transport, tt.configured)
			if got != tt.want {
				t.Errorf("EffectiveSessionReconnectMax(%q, %d) = %d, want %d",
					tt.transport, tt.configured, got, tt.want)
			}
		})
	}
}

func TestDefaultSessionReconnectMax(t *testing.T) {
	tests := []struct {
		name      string
		transport string
		want      int
	}{
		{
			name:      "stdio_returns_zero",
			transport: "stdio",
			want:      0,
		},
		{
			name:      "sse_returns_default",
			transport: "sse",
			want:      3,
		},
		{
			name:      "streamable_returns_default",
			transport: "streamable",
			want:      3,
		},
		{
			name:      "streamable_http_alias_returns_default",
			transport: "streamable_http",
			want:      3,
		},
		{
			name:      "http_alias_returns_default",
			transport: "http",
			want:      3,
		},
		{
			name:      "streamablehttp_alias_returns_default",
			transport: "streamablehttp",
			want:      3,
		},
		{
			name:      "unknown_transport_returns_zero",
			transport: "unknown",
			want:      0,
		},
		{
			name:      "empty_transport_returns_zero",
			transport: "",
			want:      0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DefaultSessionReconnectMax(tt.transport)
			if got != tt.want {
				t.Errorf("DefaultSessionReconnectMax(%q) = %d, want %d",
					tt.transport, got, tt.want)
			}
		})
	}
}
