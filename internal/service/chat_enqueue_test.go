package service

import (
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
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
		wantCode    apierror.Code
		wantBizCode string
	}{
		{"queue_full", biz.ChatEnqueueRejectQueueFull, apierror.CodeRateLimit, "CHAT_QUEUE_FULL"},
		{"no_active_run", biz.ChatEnqueueRejectNoActiveRun, apierror.CodeConflict, "CHAT_RUN_ENDED"},
		{"unknown", "unknown_reason", apierror.CodeBadRequest, "CHAT_ENQUEUE_REJECTED"},
		{"empty", "", apierror.CodeBadRequest, "CHAT_ENQUEUE_REJECTED"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := enqueueRejectError(tt.reason)
			if err == nil {
				t.Fatal("expected non-nil error")
			}
			ae, ok := apierror.From(err)
			if !ok {
				t.Fatalf("expected *apierror.Error, got %T", err)
			}
			if ae.Code != tt.wantCode {
				t.Errorf("code = %v, want %v", ae.Code, tt.wantCode)
			}
			if ae.Domain != tt.wantBizCode {
				t.Errorf("domain = %q, want %q", ae.Domain, tt.wantBizCode)
			}
		})
	}
}
