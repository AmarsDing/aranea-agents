package service

import (
	"context"
	"errors"
	"testing"
)

func TestClassifyChannelTurnError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want channelTurnErrorKind
	}{
		{"nil_error", nil, channelTurnErrNone},
		{"canceled", context.Canceled, channelTurnErrNone},
		{"busy_error", turnBusyError(), channelTurnErrBusy},
		{"turn_timeout_code", TurnError(TurnErrTurnTimeout, ""), channelTurnErrTimeout},
		{"first_byte_timeout_code", TurnError(TurnErrFirstByteTimeout, ""), channelTurnErrTimeout},
		{"deadline_exceeded", context.DeadlineExceeded, channelTurnErrTimeout},
		{"timeout_in_message", errors.New("connection timeout after 30s"), channelTurnErrTimeout},
		{"deadline_exceeded_in_message", errors.New("deadline exceeded while waiting"), channelTurnErrTimeout},
		{"rate_limit_string", errors.New("rate limit exceeded"), channelTurnErrRateLimit},
		{"too_many_requests_string", errors.New("429 Too Many Requests"), channelTurnErrRateLimit},
		{"http_429_string", errors.New("HTTP 429 error"), channelTurnErrRateLimit},
		{"context_length_string", errors.New("context length exceeded"), channelTurnErrContextOverflow},
		{"maximum_context_string", errors.New("maximum context size reached"), channelTurnErrContextOverflow},
		{"context_window_string", errors.New("context window exceeded"), channelTurnErrContextOverflow},
		{"prompt_too_long_string", errors.New("prompt is too long for model"), channelTurnErrContextOverflow},
		{"token_exceed_string", errors.New("token count exceed limit"), channelTurnErrContextOverflow},
		{"token_limit_string", errors.New("token limit reached"), channelTurnErrContextOverflow},
		{"sentinel_rate_limit", contextRateLimitSentinel{}, channelTurnErrRateLimit},
		{"generic_error", errors.New("something went wrong"), channelTurnErrGeneric},
		{"validation_error", errors.New("validation failed"), channelTurnErrGeneric},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyChannelTurnError(tt.err)
			if got != tt.want {
				t.Errorf("classifyChannelTurnError() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestClassifyChannelTurnError_TokenExceedButNoLimit(t *testing.T) {
	got := classifyChannelTurnError(errors.New("token count is 5000"))
	if got != channelTurnErrGeneric {
		t.Errorf("token without exceed/limit = %q, want generic", got)
	}
}

func TestClassifyChannelTurnError_RateLimitCaseInsensitive(t *testing.T) {
	got := classifyChannelTurnError(errors.New("RATE LIMIT hit"))
	if got != channelTurnErrRateLimit {
		t.Errorf("RATE LIMIT = %q, want rate_limit", got)
	}
}

func TestContextRateLimitSentinel(t *testing.T) {
	var err error = contextRateLimitSentinel{}
	if err.Error() != "provider rate limit exceeded" {
		t.Errorf("Error() = %q, want %q", err.Error(), "provider rate limit exceeded")
	}
	if classifyChannelTurnError(err) != channelTurnErrRateLimit {
		t.Error("sentinel should classify as rate_limit")
	}
}
