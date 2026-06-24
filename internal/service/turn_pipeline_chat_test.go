package service

import (
	"context"
	"testing"
)

func TestContextWithAdmittedTurnID(t *testing.T) {
	tests := []struct {
		name   string
		turnID string
		want   string
	}{
		{"normal", "turn-123", "turn-123"},
		{"whitespace_trimmed", "  turn-456  ", "turn-456"},
		{"empty_returns_original_ctx", "", ""},
		{"whitespace_only_returns_original_ctx", "   ", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := contextWithAdmittedTurnID(context.Background(), tt.turnID)
			got := admittedTurnIDFromContext(ctx)
			if got != tt.want {
				t.Errorf("admittedTurnIDFromContext() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAdmittedTurnIDFromContext(t *testing.T) {
	t.Run("nil_context", func(t *testing.T) {
		got := admittedTurnIDFromContext(nil)
		if got != "" {
			t.Errorf("nil context should return empty, got %q", got)
		}
	})

	t.Run("no_value", func(t *testing.T) {
		got := admittedTurnIDFromContext(context.Background())
		if got != "" {
			t.Errorf("background context should return empty, got %q", got)
		}
	})

	t.Run("wrong_type", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), admittedTurnContextKey{}, 12345)
		got := admittedTurnIDFromContext(ctx)
		if got != "" {
			t.Errorf("wrong type should return empty, got %q", got)
		}
	})

	t.Run("roundtrip", func(t *testing.T) {
		ctx := contextWithAdmittedTurnID(context.Background(), "turn-abc")
		got := admittedTurnIDFromContext(ctx)
		if got != "turn-abc" {
			t.Errorf("roundtrip = %q, want %q", got, "turn-abc")
		}
	})
}
