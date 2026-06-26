package service

import (
	"context"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// ProvideSessionLogWriter adapts a loggateway.Logger to biz.SessionLogWriter
// so biz consumers don't depend on loggateway directly for session-scoped logs.
func ProvideSessionLogWriter(lg loggateway.Logger) biz.SessionLogWriter {
	if lg == nil {
		return nil
	}
	return sessionLogWriterAdapter{lg: lg}
}

type sessionLogWriterAdapter struct {
	lg loggateway.Logger
}

func (a sessionLogWriterAdapter) LogSessionWarn(ctx context.Context, sessionID, stepID, message string, pairs ...biz.LogPair) {
	a.lg.Warn(message, appendSessionFields(sessionID, stepID, pairs)...)
}

func (a sessionLogWriterAdapter) LogSessionError(ctx context.Context, sessionID, stepID, message string, pairs ...biz.LogPair) {
	a.lg.Error(message, appendSessionFields(sessionID, stepID, pairs)...)
}

// ProvideSystemLogWriter adapts a loggateway.Logger to biz.SystemLogWriter
// so biz consumers don't depend on loggateway directly for system-scoped logs.
func ProvideSystemLogWriter(lg loggateway.Logger) biz.SystemLogWriter {
	if lg == nil {
		return nil
	}
	return systemLogWriterAdapter{lg: lg}
}

type systemLogWriterAdapter struct {
	lg loggateway.Logger
}

func (a systemLogWriterAdapter) LogWarn(stepID, message string, pairs ...biz.LogPair) {
	a.lg.Warn(message, appendSystemFields(stepID, pairs)...)
}

func (a systemLogWriterAdapter) LogError(stepID, message string, pairs ...biz.LogPair) {
	a.lg.Error(message, appendSystemFields(stepID, pairs)...)
}

// appendSessionFields converts biz.LogPair slices to loggateway field options,
// prepending session_id and step_id for structured session-scoped logs.
func appendSessionFields(sessionID, stepID string, pairs []biz.LogPair) []loggateway.Field {
	fields := make([]loggateway.Field, 0, len(pairs)+2)
	fields = append(fields, loggateway.SessionID(sessionID), loggateway.StepID(stepID))
	for _, p := range pairs {
		fields = append(fields, loggateway.Any(p.Key, p.Value))
	}
	return fields
}

// appendSystemFields converts biz.LogPair slices to loggateway field options,
// prepending step_id for structured system-scoped logs.
func appendSystemFields(stepID string, pairs []biz.LogPair) []loggateway.Field {
	fields := make([]loggateway.Field, 0, len(pairs)+1)
	fields = append(fields, loggateway.StepID(stepID))
	for _, p := range pairs {
		fields = append(fields, loggateway.Any(p.Key, p.Value))
	}
	return fields
}
