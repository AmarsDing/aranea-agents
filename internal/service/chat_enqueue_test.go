package service

import (
	"testing"

	"aranea-agents/internal/biz"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

func TestEnqueueRejectMessage(t *testing.T) {
	tests := []struct {
		name   string
		reason string
		want   string
	}{
		{"queue_full", biz.ChatEnqueueRejectQueueFull, "pending queue is full for this session"},
		{"no_active_run", biz.ChatEnqueueRejectNoActiveRun, "agent run has ended; send your message again to start a new turn"},
		{"unknown", "unknown_reason", "message could not be queued"},
		{"empty", "", "message could not be queued"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := enqueueRejectMessage(tt.reason)
			if got != tt.want {
				t.Errorf("enqueueRejectMessage(%q) = %q, want %q", tt.reason, got, tt.want)
			}
		})
	}
}

func TestEnqueueRejectError(t *testing.T) {
	tests := []struct {
		name        string
		reason      string
		wantCode    int32
		wantBizCode string
	}{
		{"queue_full", biz.ChatEnqueueRejectQueueFull, 400, "CHAT_QUEUE_FULL"},
		{"no_active_run", biz.ChatEnqueueRejectNoActiveRun, 409, "CHAT_RUN_ENDED"},
		{"unknown", "unknown_reason", 400, "CHAT_ENQUEUE_REJECTED"},
		{"empty", "", 400, "CHAT_ENQUEUE_REJECTED"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := enqueueRejectError(tt.reason)
			if err == nil {
				t.Fatal("expected non-nil error")
			}
			ke, ok := err.(*kerrors.Error)
			if !ok {
				t.Fatalf("expected *kerrors.Error, got %T", err)
			}
			if ke.Code != tt.wantCode {
				t.Errorf("code = %d, want %d", ke.Code, tt.wantCode)
			}
			if ke.Reason != tt.wantBizCode {
				t.Errorf("reason = %q, want %q", ke.Reason, tt.wantBizCode)
			}
		})
	}
}
