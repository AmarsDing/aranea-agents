package service

import (
	"context"
	"errors"
	"testing"

	"aranea-agents/internal/biz"
)

func TestClassifyChannelTurnError_nil(t *testing.T) {
	got := classifyChannelTurnError(nil)
	if got != biz.ChannelTurnErrNone {
		t.Errorf("classifyChannelTurnError(nil) = %q, want %q", got, biz.ChannelTurnErrNone)
	}
}

func TestClassifyChannelTurnError_canceled(t *testing.T) {
	got := classifyChannelTurnError(context.Canceled)
	if got != biz.ChannelTurnErrNone {
		t.Errorf("classifyChannelTurnError(Canceled) = %q, want %q", got, biz.ChannelTurnErrNone)
	}
}

func TestClassifyChannelTurnError_busy(t *testing.T) {
	got := classifyChannelTurnError(turnBusyError())
	if got != biz.ChannelTurnErrBusy {
		t.Errorf("classifyChannelTurnError(busy) = %q, want %q", got, biz.ChannelTurnErrBusy)
	}
}

func TestClassifyChannelTurnError_turnTimeout(t *testing.T) {
	got := classifyChannelTurnError(TurnError(TurnErrTurnTimeout, "5m"))
	if got != biz.ChannelTurnErrTimeout {
		t.Errorf("classifyChannelTurnError(TurnTimeout) = %q, want %q", got, biz.ChannelTurnErrTimeout)
	}
}

func TestClassifyChannelTurnError_firstByteTimeout(t *testing.T) {
	got := classifyChannelTurnError(TurnError(TurnErrFirstByteTimeout, "30s"))
	if got != biz.ChannelTurnErrTimeout {
		t.Errorf("classifyChannelTurnError(FirstByteTimeout) = %q, want %q", got, biz.ChannelTurnErrTimeout)
	}
}

func TestClassifyChannelTurnError_deadlineExceeded(t *testing.T) {
	got := classifyChannelTurnError(context.DeadlineExceeded)
	if got != biz.ChannelTurnErrTimeout {
		t.Errorf("classifyChannelTurnError(DeadlineExceeded) = %q, want %q", got, biz.ChannelTurnErrTimeout)
	}
}

func TestClassifyChannelTurnError_timeoutString(t *testing.T) {
	// String-based timeout detection removed; plain errors are now classified as generic.
	got := classifyChannelTurnError(errors.New("connection timeout after 30s"))
	if got != biz.ChannelTurnErrGeneric {
		t.Errorf("classifyChannelTurnError(timeout string) = %q, want %q", got, biz.ChannelTurnErrGeneric)
	}
}

func TestClassifyChannelTurnError_rateLimit(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{"rate_limit", errors.New("rate limit exceeded")},
		{"too_many_requests", errors.New("429 Too Many Requests")},
		{"429_code", errors.New("HTTP 429")},
		{"sentinel", contextRateLimitSentinel{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyChannelTurnError(tt.err)
			if got != biz.ChannelTurnErrRateLimit {
				t.Errorf("classifyChannelTurnError() = %q, want %q", got, biz.ChannelTurnErrRateLimit)
			}
		})
	}
}

func TestClassifyChannelTurnError_contextOverflow(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{"context_length", errors.New("context length exceeded")},
		{"maximum_context", errors.New("maximum context size reached")},
		{"context_window", errors.New("context window exceeded")},
		{"prompt_too_long", errors.New("prompt is too long for model")},
		{"token_exceed_limit", errors.New("token count exceed limit")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyChannelTurnError(tt.err)
			if got != biz.ChannelTurnErrContextOverflow {
				t.Errorf("classifyChannelTurnError() = %q, want %q", got, biz.ChannelTurnErrContextOverflow)
			}
		})
	}
}

func TestClassifyChannelTurnError_generic(t *testing.T) {
	got := classifyChannelTurnError(errors.New("internal sql: connection refused"))
	if got != biz.ChannelTurnErrGeneric {
		t.Errorf("classifyChannelTurnError(generic) = %q, want %q", got, biz.ChannelTurnErrGeneric)
	}
}

func TestClassifyChannelTurnError_rateLimitNotTimeout(t *testing.T) {
	err := errors.New("rate limit exceeded")
	got := classifyChannelTurnError(err)
	if got != biz.ChannelTurnErrRateLimit {
		t.Errorf("rate limit should not be classified as timeout, got %q", got)
	}
}

func TestContextRateLimitSentinel(t *testing.T) {
	var err error = contextRateLimitSentinel{}
	if err.Error() != "provider rate limit exceeded" {
		t.Errorf("Error() = %q, want %q", err.Error(), "provider rate limit exceeded")
	}
}
