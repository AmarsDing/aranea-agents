package service

import (
	"context"
	"errors"
	"testing"
)

func TestClassifyChannelTurnError_nil(t *testing.T) {
	got := classifyChannelTurnError(nil)
	if got != channelTurnErrNone {
		t.Errorf("classifyChannelTurnError(nil) = %q, want %q", got, channelTurnErrNone)
	}
}

func TestClassifyChannelTurnError_canceled(t *testing.T) {
	got := classifyChannelTurnError(context.Canceled)
	if got != channelTurnErrNone {
		t.Errorf("classifyChannelTurnError(Canceled) = %q, want %q", got, channelTurnErrNone)
	}
}

func TestClassifyChannelTurnError_busy(t *testing.T) {
	got := classifyChannelTurnError(turnBusyError())
	if got != channelTurnErrBusy {
		t.Errorf("classifyChannelTurnError(busy) = %q, want %q", got, channelTurnErrBusy)
	}
}

func TestClassifyChannelTurnError_turnTimeout(t *testing.T) {
	got := classifyChannelTurnError(TurnError(TurnErrTurnTimeout, "5m"))
	if got != channelTurnErrTimeout {
		t.Errorf("classifyChannelTurnError(TurnTimeout) = %q, want %q", got, channelTurnErrTimeout)
	}
}

func TestClassifyChannelTurnError_firstByteTimeout(t *testing.T) {
	got := classifyChannelTurnError(TurnError(TurnErrFirstByteTimeout, "30s"))
	if got != channelTurnErrTimeout {
		t.Errorf("classifyChannelTurnError(FirstByteTimeout) = %q, want %q", got, channelTurnErrTimeout)
	}
}

func TestClassifyChannelTurnError_deadlineExceeded(t *testing.T) {
	got := classifyChannelTurnError(context.DeadlineExceeded)
	if got != channelTurnErrTimeout {
		t.Errorf("classifyChannelTurnError(DeadlineExceeded) = %q, want %q", got, channelTurnErrTimeout)
	}
}

func TestClassifyChannelTurnError_timeoutString(t *testing.T) {
	got := classifyChannelTurnError(errors.New("connection timeout after 30s"))
	if got != channelTurnErrTimeout {
		t.Errorf("classifyChannelTurnError(timeout string) = %q, want %q", got, channelTurnErrTimeout)
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
			if got != channelTurnErrRateLimit {
				t.Errorf("classifyChannelTurnError() = %q, want %q", got, channelTurnErrRateLimit)
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
			if got != channelTurnErrContextOverflow {
				t.Errorf("classifyChannelTurnError() = %q, want %q", got, channelTurnErrContextOverflow)
			}
		})
	}
}

func TestClassifyChannelTurnError_generic(t *testing.T) {
	got := classifyChannelTurnError(errors.New("internal sql: connection refused"))
	if got != channelTurnErrGeneric {
		t.Errorf("classifyChannelTurnError(generic) = %q, want %q", got, channelTurnErrGeneric)
	}
}

func TestClassifyChannelTurnError_rateLimitNotTimeout(t *testing.T) {
	err := errors.New("rate limit exceeded")
	got := classifyChannelTurnError(err)
	if got != channelTurnErrRateLimit {
		t.Errorf("rate limit should not be classified as timeout, got %q", got)
	}
}

func TestContextRateLimitSentinel(t *testing.T) {
	var err error = contextRateLimitSentinel{}
	if err.Error() != "provider rate limit exceeded" {
		t.Errorf("Error() = %q, want %q", err.Error(), "provider rate limit exceeded")
	}
}
