package service

import (
	"context"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/internal/event/contract"
)

// --- EnvelopeBuffer adapter ---

// envelopeBufferAdapter adapts event.Buffer to biz.EnvelopeBuffer.
type envelopeBufferAdapter struct {
	buf *event.Buffer
}

func (a envelopeBufferAdapter) Append(env contract.Envelope) {
	if a.buf != nil {
		a.buf.Append(env)
	}
}

// ProvideEnvelopeBuffer creates a biz.EnvelopeBuffer backed by event.Buffer.
func ProvideEnvelopeBuffer(buf *event.Buffer) biz.EnvelopeBuffer {
	return envelopeBufferAdapter{buf: buf}
}

// --- SessionLogWriter adapter ---

// sessionLogWriterAdapter adapts event session log functions to biz.SessionLogWriter.
type sessionLogWriterAdapter struct{}

func (a sessionLogWriterAdapter) SessionSysLogWarn(ctx context.Context, sessionID, stepID, message string, pairs ...biz.LogPair) {
	event.SessionSysLogWarn(ctx, sessionID, stepID, message, toEventPairs(pairs)...)
}

func (a sessionLogWriterAdapter) SessionSysLogError(ctx context.Context, sessionID, stepID, message string, pairs ...biz.LogPair) {
	event.SessionSysLogError(ctx, sessionID, stepID, message, toEventPairs(pairs)...)
}

// ProvideSessionLogWriter creates a biz.SessionLogWriter backed by event session log functions.
func ProvideSessionLogWriter() biz.SessionLogWriter {
	return sessionLogWriterAdapter{}
}

// --- SystemLogWriter adapter ---

// systemLogWriterAdapter adapts event system log functions to biz.SystemLogWriter.
type systemLogWriterAdapter struct{}

func (a systemLogWriterAdapter) SysLogWarn(stepID, message string, pairs ...biz.LogPair) {
	event.SysLogWarn(stepID, message, toEventPairs(pairs)...)
}

func (a systemLogWriterAdapter) SysLogError(stepID, message string, pairs ...biz.LogPair) {
	event.SysLogError(stepID, message, toEventPairs(pairs)...)
}

// ProvideSystemLogWriter creates a biz.SystemLogWriter backed by event system log functions.
func ProvideSystemLogWriter() biz.SystemLogWriter {
	return systemLogWriterAdapter{}
}

// --- helpers ---

func toEventPairs(pairs []biz.LogPair) []event.Pair {
	out := make([]event.Pair, len(pairs))
	for i, p := range pairs {
		out[i] = event.P(p.Key, p.Value)
	}
	return out
}
