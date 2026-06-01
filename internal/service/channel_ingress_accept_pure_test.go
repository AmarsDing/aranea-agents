package service

import (
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/channel/port"
)

func TestIsPureAsyncExecutionMode(t *testing.T) {
	tests := []struct {
		name string
		cfg  biz.ChannelLongTaskConfig
		want bool
	}{
		{"async_lowercase", biz.ChannelLongTaskConfig{ExecutionMode: "async"}, true},
		{"async_uppercase", biz.ChannelLongTaskConfig{ExecutionMode: "ASYNC"}, true},
		{"async_mixed_case", biz.ChannelLongTaskConfig{ExecutionMode: "Async"}, true},
		{"async_with_whitespace", biz.ChannelLongTaskConfig{ExecutionMode: "  async  "}, true},
		{"sync", biz.ChannelLongTaskConfig{ExecutionMode: "sync"}, false},
		{"auto", biz.ChannelLongTaskConfig{ExecutionMode: "auto"}, false},
		{"empty", biz.ChannelLongTaskConfig{ExecutionMode: ""}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isPureAsyncExecutionMode(tt.cfg); got != tt.want {
				t.Errorf("isPureAsyncExecutionMode() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAckIdempotencyKey(t *testing.T) {
	tests := []struct {
		name     string
		platform string
		ev       port.InboundEvent
		suffix   string
		want     string
	}{
		{
			name:     "with_idempotency_key",
			platform: "feishu",
			ev:       port.InboundEvent{IdempotencyKey: "msg-123", PeerID: "ou_x"},
			suffix:   "ack",
			want:     "msg-123:ack",
		},
		{
			name:     "without_idempotency_key",
			platform: "feishu",
			ev:       port.InboundEvent{IdempotencyKey: "", PeerID: "ou_x"},
			suffix:   "ack",
			want:     "feishu:ou_x:ack",
		},
		{
			name:     "whitespace_idempotency_key",
			platform: "feishu",
			ev:       port.InboundEvent{IdempotencyKey: "   ", PeerID: "ou_x"},
			suffix:   "ack",
			want:     "feishu:ou_x:ack",
		},
		{
			name:     "error_suffix",
			platform: "slack",
			ev:       port.InboundEvent{IdempotencyKey: "msg-456", PeerID: "U123"},
			suffix:   "error",
			want:     "msg-456:error",
		},
		{
			name:     "concurrent_busy_suffix",
			platform: "feishu",
			ev:       port.InboundEvent{IdempotencyKey: "", PeerID: "ou_y"},
			suffix:   "concurrent_busy",
			want:     "feishu:ou_y:concurrent_busy",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ackIdempotencyKey(tt.platform, tt.ev, tt.suffix)
			if got != tt.want {
				t.Errorf("ackIdempotencyKey() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestChannelAcceptOutcomeFromRoute_SyncVsAsync(t *testing.T) {
	asyncOutcome := channelAcceptOutcomeFromRoute(IngressPolicyResult{Decision: IngressRouteAsync})
	if !asyncOutcome.DispatchAsync || asyncOutcome.ExecuteSync {
		t.Errorf("async route: DispatchAsync=%v ExecuteSync=%v", asyncOutcome.DispatchAsync, asyncOutcome.ExecuteSync)
	}

	syncOutcome := channelAcceptOutcomeFromRoute(IngressPolicyResult{Decision: IngressAdmit})
	if syncOutcome.DispatchAsync || !syncOutcome.ExecuteSync {
		t.Errorf("admit route: DispatchAsync=%v ExecuteSync=%v", syncOutcome.DispatchAsync, syncOutcome.ExecuteSync)
	}
}
