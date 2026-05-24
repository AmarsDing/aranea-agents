package service

import (
	"errors"
	"strings"
)

type channelTurnErrorKind string

const (
	channelTurnErrNone            channelTurnErrorKind = ""
	channelTurnErrBusy            channelTurnErrorKind = "busy"
	channelTurnErrTimeout         channelTurnErrorKind = "timeout"
	channelTurnErrRateLimit       channelTurnErrorKind = "rate_limit"
	channelTurnErrContextOverflow channelTurnErrorKind = "context_overflow"
	channelTurnErrGeneric         channelTurnErrorKind = "generic"
)

const (
	channelTurnErrorRateLimitMsg    = "模型调用过于频繁，请稍后再试。"
	channelTurnErrorContextOverflow = "对话上下文过长，请发送 /background 转入后台继续，或在 Web 端查看。"
)

// classifyChannelTurnError maps provider/runtime failures to IM-safe taxonomy (CH-BOR-09).
func classifyChannelTurnError(err error) channelTurnErrorKind {
	if err == nil || turnErrorIsCanceled(err) {
		return channelTurnErrNone
	}
	if IsTurnBusyError(err) {
		return channelTurnErrBusy
	}
	switch TurnErrorCodeFromErr(err) {
	case TurnErrTurnTimeout, TurnErrFirstByteTimeout:
		return channelTurnErrTimeout
	}
	if turnErrorIsTimeout(err) {
		return channelTurnErrTimeout
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "rate limit") ||
		strings.Contains(msg, "too many requests") ||
		strings.Contains(msg, "429") ||
		errors.Is(err, contextRateLimitSentinel{}) {
		return channelTurnErrRateLimit
	}
	if strings.Contains(msg, "context length") ||
		strings.Contains(msg, "maximum context") ||
		strings.Contains(msg, "context window") ||
		strings.Contains(msg, "prompt is too long") ||
		strings.Contains(msg, "token") && (strings.Contains(msg, "exceed") || strings.Contains(msg, "limit")) {
		return channelTurnErrContextOverflow
	}
	return channelTurnErrGeneric
}

// contextRateLimitSentinel allows transports to signal rate limits without string matching.
type contextRateLimitSentinel struct{}

func (contextRateLimitSentinel) Error() string { return "provider rate limit exceeded" }
