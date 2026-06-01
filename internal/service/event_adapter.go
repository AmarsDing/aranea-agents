package service

import (
	"context"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/internal/event/contract"
	"aranea-agents/pkg/loggateway"
)

// --- EnvelopeBuffer adapter ---

type envelopeBufferAdapter struct {
	buf *event.Buffer
}

func (a envelopeBufferAdapter) Append(env contract.Envelope) {
	if a.buf != nil {
		a.buf.Append(env)
	}
}

func ProvideEnvelopeBuffer(buf *event.Buffer) biz.EnvelopeBuffer {
	return envelopeBufferAdapter{buf: buf}
}

// --- SessionLogWriter adapter ---

type sessionLogWriterAdapter struct {
	lg loggateway.Logger
}

func (a sessionLogWriterAdapter) SessionSysLogWarn(ctx context.Context, sessionID, stepID, message string, pairs ...biz.LogPair) {
	a.lg.With(loggateway.SessionID(sessionID)).Warn(message,
		loggateway.StepID(stepID),
	)
}

func (a sessionLogWriterAdapter) SessionSysLogError(ctx context.Context, sessionID, stepID, message string, pairs ...biz.LogPair) {
	a.lg.With(loggateway.SessionID(sessionID)).Error(message,
		loggateway.StepID(stepID),
	)
}

func ProvideSessionLogWriter(lg loggateway.Logger) biz.SessionLogWriter {
	return sessionLogWriterAdapter{lg: lg}
}

// --- SystemLogWriter adapter ---

type systemLogWriterAdapter struct {
	lg loggateway.Logger
}

func (a systemLogWriterAdapter) SysLogWarn(stepID, message string, pairs ...biz.LogPair) {
	a.lg.Warn(message,
		loggateway.StepID(stepID),
	)
}

func (a systemLogWriterAdapter) SysLogError(stepID, message string, pairs ...biz.LogPair) {
	a.lg.Error(message,
		loggateway.StepID(stepID),
	)
}

func ProvideSystemLogWriter(lg loggateway.Logger) biz.SystemLogWriter {
	return systemLogWriterAdapter{lg: lg}
}

// --- helpers ---

func toEventPairs(pairs []biz.LogPair) []event.Pair {
	out := make([]event.Pair, len(pairs))
	for i, p := range pairs {
		out[i] = event.P(p.Key, p.Value)
	}
	return out
}
