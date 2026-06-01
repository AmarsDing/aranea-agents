package service

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
)

func TestTurnResultFromNative(t *testing.T) {
	tests := []struct {
		name   string
		input  biz.NativeTurnResult
		want   biz.TurnResult
	}{
		{
			name: "completed",
			input: biz.NativeTurnResult{
				Outcome: biz.NativeTurnOutcomeCompleted,
				UserMsg: biz.ChatMessage{ID: "u1", ContentMarkdown: "hello"},
				AssistantMsg: biz.ChatMessage{ID: "a1", ContentMarkdown: "world"},
			},
			want: biz.TurnResult{
				Outcome:      biz.TurnOutcomeCompleted,
				UserMsg:      biz.ChatMessage{ID: "u1", ContentMarkdown: "hello"},
				AssistantMsg: biz.ChatMessage{ID: "a1", ContentMarkdown: "world"},
				Reply:        "world",
			},
		},
		{
			name: "queued",
			input: biz.NativeTurnResult{
				Outcome:   biz.NativeTurnOutcomeQueued,
				PendingID: "pend-1",
			},
			want: biz.TurnResult{
				Outcome:   biz.TurnOutcomeQueued,
				PendingID: "pend-1",
			},
		},
		{
			name: "failed",
			input: biz.NativeTurnResult{
				Outcome: biz.NativeTurnOutcomeFailed,
				UserMsg: biz.ChatMessage{ID: "u2", ContentMarkdown: "hello"},
			},
			want: biz.TurnResult{
				Outcome: biz.TurnOutcomeFailed,
				UserMsg: biz.ChatMessage{ID: "u2", ContentMarkdown: "hello"},
			},
		},
		{
			name: "unknown_outcome_defaults_to_failed",
			input: biz.NativeTurnResult{
				Outcome: "unknown",
			},
			want: biz.TurnResult{
				Outcome: biz.TurnOutcomeFailed,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := turnResultFromNative(tt.input)
			if got.Outcome != tt.want.Outcome {
				t.Errorf("Outcome = %q, want %q", got.Outcome, tt.want.Outcome)
			}
			if got.Reply != tt.want.Reply {
				t.Errorf("Reply = %q, want %q", got.Reply, tt.want.Reply)
			}
			if got.PendingID != tt.want.PendingID {
				t.Errorf("PendingID = %q, want %q", got.PendingID, tt.want.PendingID)
			}
			if got.UserMsg.ID != tt.want.UserMsg.ID {
				t.Errorf("UserMsg.ID = %q, want %q", got.UserMsg.ID, tt.want.UserMsg.ID)
			}
			if got.AssistantMsg.ID != tt.want.AssistantMsg.ID {
				t.Errorf("AssistantMsg.ID = %q, want %q", got.AssistantMsg.ID, tt.want.AssistantMsg.ID)
			}
		})
	}
}

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
