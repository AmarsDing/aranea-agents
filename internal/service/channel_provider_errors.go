package service

import (
	"errors"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/provider"
)

// classifyChannelTurnError maps provider/runtime failures to IM-safe taxonomy (CH-BOR-09).
func classifyChannelTurnError(err error) biz.ChannelTurnErrorKind {
	if err == nil || turnErrorIsCanceled(err) {
		return biz.ChannelTurnErrNone
	}
	if isTurnBusyError(err) {
		return biz.ChannelTurnErrBusy
	}
	switch TurnErrorCodeFromErr(err) {
	case TurnErrTurnTimeout, TurnErrFirstByteTimeout:
		return biz.ChannelTurnErrTimeout
	case TurnErrProviderBilling:
		return biz.ChannelTurnErrBilling
	case TurnErrProviderAuth:
		return biz.ChannelTurnErrAuth
	}
	fail := provider.ClassifyFailure(err.Error(), err)
	switch fail.Kind {
	case provider.FailureBilling:
		return biz.ChannelTurnErrBilling
	case provider.FailureAuth:
		return biz.ChannelTurnErrAuth
	case provider.FailureStall:
		return biz.ChannelTurnErrTimeout
	}
	if turnErrorIsTimeout(err) {
		return biz.ChannelTurnErrTimeout
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "rate limit") ||
		strings.Contains(msg, "too many requests") ||
		strings.Contains(msg, "429") ||
		errors.Is(err, contextRateLimitSentinel{}) {
		return biz.ChannelTurnErrRateLimit
	}
	if strings.Contains(msg, "context length") ||
		strings.Contains(msg, "maximum context") ||
		strings.Contains(msg, "context window") ||
		strings.Contains(msg, "prompt is too long") ||
		strings.Contains(msg, "token") && (strings.Contains(msg, "exceed") || strings.Contains(msg, "limit")) {
		return biz.ChannelTurnErrContextOverflow
	}
	return biz.ChannelTurnErrGeneric
}

// contextRateLimitSentinel allows transports to signal rate limits without string matching.
type contextRateLimitSentinel struct{}

func (contextRateLimitSentinel) Error() string { return "provider rate limit exceeded" }
