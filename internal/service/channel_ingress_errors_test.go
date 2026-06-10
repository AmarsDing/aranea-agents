package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"aranea-agents/internal/biz"
)

func TestFormatChannelTurnErrorMessage(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"nil_error", nil, ""},
		{"canceled", context.Canceled, ""},
		{"busy", turnBusyError(), biz.ChannelTurnErrorBusyMsg},
		{"deadline_exceeded", context.DeadlineExceeded, biz.ChannelTurnErrorSyncCapMsg},
		{"turn_timeout_code", TurnError(TurnErrTurnTimeout, "5m"), biz.ChannelTurnErrorSyncCapMsg},
		{"first_byte_timeout_code", TurnError(TurnErrFirstByteTimeout, "30s"), biz.ChannelTurnErrorSyncCapMsg},
		{"timeout_string", errors.New("connection timeout after 30s"), biz.ChannelTurnErrorGenericMsg},
		{"rate_limit_string", errors.New("rate limit exceeded"), biz.ChannelTurnErrorRateLimitMsg},
		{"too_many_requests_string", errors.New("429 Too Many Requests"), biz.ChannelTurnErrorRateLimitMsg},
		{"context_length_string", errors.New("context length exceeded"), biz.ChannelTurnErrorContextOverflowMsg},
		{"maximum_context_string", errors.New("maximum context size reached"), biz.ChannelTurnErrorContextOverflowMsg},
		{"context_window_string", errors.New("context window exceeded"), biz.ChannelTurnErrorContextOverflowMsg},
		{"prompt_too_long_string", errors.New("prompt is too long for model"), biz.ChannelTurnErrorContextOverflowMsg},
		{"token_exceed_string", errors.New("token count exceed limit"), biz.ChannelTurnErrorContextOverflowMsg},
		{"generic_error", errors.New("internal sql: connection refused"), biz.ChannelTurnErrorGenericMsg},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatChannelTurnErrorMessage(tt.err)
			if got != tt.want {
				t.Errorf("formatChannelTurnErrorMessage() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFormatChannelTurnErrorMessage_NoLeakInternalError(t *testing.T) {
	msg := formatChannelTurnErrorMessage(errors.New("internal sql: connection refused"))
	if strings.Contains(msg, "sql") {
		t.Fatal("must not leak internal error text to IM")
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
		{"canceled_wrapped", errors.Join(context.Canceled, errors.New("extra")), true},
		{"deadline_exceeded", context.DeadlineExceeded, false},
		{"generic", errors.New("something"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := turnErrorIsCanceled(tt.err); got != tt.want {
				t.Errorf("turnErrorIsCanceled() = %v, want %v", got, tt.want)
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
		{"canceled", context.Canceled, false},
		{"deadline_exceeded", context.DeadlineExceeded, true},
		{"deadline_exceeded_wrapped", errors.Join(context.DeadlineExceeded, errors.New("extra")), true},
		{"generic", errors.New("validation failed"), false},
		{"rate_limit", errors.New("rate limit exceeded"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := turnErrorIsTimeout(tt.err); got != tt.want {
				t.Errorf("turnErrorIsTimeout() = %v, want %v", got, tt.want)
			}
		})
	}
}
