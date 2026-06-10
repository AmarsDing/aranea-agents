package service_test

import (
	"context"
	"errors"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/service"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

func TestEnqueueRejectMessage(t *testing.T) {
	tests := []struct {
		name   string
		reason string
		want   string
	}{
		{
			name:   "queue_full",
			reason: biz.ChatEnqueueRejectQueueFull,
			want:   "pending queue is full for this session",
		},
		{
			name:   "no_active_run",
			reason: biz.ChatEnqueueRejectNoActiveRun,
			want:   "agent run has ended; send your message again to start a new turn",
		},
		{
			name:   "unknown_reason",
			reason: "something_else",
			want:   "message could not be queued",
		},
		{
			name:   "empty_reason",
			reason: "",
			want:   "message could not be queued",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := service.EnqueueRejectMessage(tt.reason); got != tt.want {
				t.Errorf("EnqueueRejectMessage(%q) = %q, want %q", tt.reason, got, tt.want)
			}
		})
	}
}

func TestEnqueueRejectError(t *testing.T) {
	tests := []struct {
		name        string
		reason      string
		wantReason  int32
		wantMessage string
	}{
		{
			name:        "queue_full_bad_request",
			reason:      biz.ChatEnqueueRejectQueueFull,
			wantReason:  400,
			wantMessage: "pending queue is full for this session",
		},
		{
			name:        "no_active_run_conflict",
			reason:      biz.ChatEnqueueRejectNoActiveRun,
			wantReason:  409,
			wantMessage: "agent run has ended; send your message again to start a new turn",
		},
		{
			name:        "unknown_reason_bad_request",
			reason:      "unknown",
			wantReason:  400,
			wantMessage: "message could not be queued",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := service.EnqueueRejectError(tt.reason)
			if got == nil {
				t.Fatal("EnqueueRejectError() returned nil")
			}
			ke := kerrors.FromError(got)
			if ke.Code != tt.wantReason {
				t.Errorf("code = %d, want %d", ke.Code, tt.wantReason)
			}
			if ke.Message != tt.wantMessage {
				t.Errorf("message = %q, want %q", ke.Message, tt.wantMessage)
			}
		})
	}
}

func TestFormatChannelTurnErrorMessage(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"nil", nil, ""},
		{"canceled", context.Canceled, ""},
		{"generic", errors.New("something broke"), "任务执行失败，请稍后重试。"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := service.FormatChannelTurnErrorMessage(tt.err); got != tt.want {
				t.Errorf("FormatChannelTurnErrorMessage() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTurnErrorIsCanceled(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"canceled", context.Canceled, true},
		{"wrapped_canceled", errors.Join(context.Canceled, errors.New("extra")), true},
		{"deadline", context.DeadlineExceeded, false},
		{"generic", errors.New("x"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := service.TurnErrorIsCanceled(tt.err); got != tt.want {
				t.Errorf("TurnErrorIsCanceled() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTurnErrorIsTimeout(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"canceled_not_timeout", context.Canceled, false},
		{"deadline_exceeded", context.DeadlineExceeded, true},
		{"timeout_string", errors.New("connection timeout"), false},
		{"deadline_string", errors.New("context deadline exceeded"), false},
		{"generic", errors.New("validation failed"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := service.TurnErrorIsTimeout(tt.err); got != tt.want {
				t.Errorf("TurnErrorIsTimeout() = %v, want %v", got, tt.want)
			}
		})
	}
}
