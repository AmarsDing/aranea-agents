package service

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/monitor"
	"aranea-agents/internal/biz/session"
	"aranea-agents/internal/event"
	"aranea-agents/internal/event/contract"
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

// ProvideFlowLogWriter adapts a loggateway.Logger + MonitorBus to
// biz.FlowLogWriter so biz-layer processes (event bus consumers, monitor,
// webhook dispatcher, session usecase) can emit user-visible flow logs
// (流程日志) without importing internal/event. Returns nil when either
// dependency is missing (tests), callers must nil-check.
func ProvideFlowLogWriter(lg loggateway.Logger, bus contract.MonitorBus) biz.FlowLogWriter {
	if lg == nil || bus == nil {
		return nil
	}
	return flowLogWriterAdapter{lg: lg, infra: event.NewInfraFromBus(bus)}
}

type flowLogWriterAdapter struct {
	lg    loggateway.Logger
	infra *event.Infra
}

func (a flowLogWriterAdapter) LogFlowStart(ctx context.Context, sessionID, stepID, message string, pairs ...biz.LogPair) {
	a.emitter(ctx, sessionID, stepID).LogStart(stepID, message, flowPairs(pairs)...)
}

func (a flowLogWriterAdapter) LogFlowDone(ctx context.Context, sessionID, stepID, message string, pairs ...biz.LogPair) {
	a.emitter(ctx, sessionID, stepID).LogDone(stepID, message, flowPairs(pairs)...)
}

func (a flowLogWriterAdapter) LogFlowError(ctx context.Context, sessionID, stepID, message string, pairs ...biz.LogPair) {
	a.emitter(ctx, sessionID, stepID).LogError(stepID, message, flowPairs(pairs)...)
}

func (a flowLogWriterAdapter) emitter(ctx context.Context, sessionID, stepID string) *event.TraceEmitter {
	return event.NewTraceEmitterForRun(event.TraceEmitterOpts{
		Ctx:       ctx,
		SessionID: sessionID,
		Domain:    domainForStepID(stepID),
		LG:        a.lg,
		Infra:     a.infra,
	})
}

// ProvideSessionFlowLogWriter adapts the shared biz.FlowLogWriter to the
// session package's mirror port (session.FlowLogWriter) so biz/session does
// not import internal/biz (re-export import cycle). Returns nil when inner
// is nil (tests), callers must nil-check.
func ProvideSessionFlowLogWriter(inner biz.FlowLogWriter) session.FlowLogWriter {
	if inner == nil {
		return nil
	}
	return sessionFlowLogWriter{inner: inner}
}

type sessionFlowLogWriter struct {
	inner biz.FlowLogWriter
}

func (w sessionFlowLogWriter) LogFlowStart(ctx context.Context, sessionID, stepID, message string, pairs ...session.LogPair) {
	w.inner.LogFlowStart(ctx, sessionID, stepID, message, sessionFlowPairs(pairs)...)
}

func (w sessionFlowLogWriter) LogFlowDone(ctx context.Context, sessionID, stepID, message string, pairs ...session.LogPair) {
	w.inner.LogFlowDone(ctx, sessionID, stepID, message, sessionFlowPairs(pairs)...)
}

func (w sessionFlowLogWriter) LogFlowError(ctx context.Context, sessionID, stepID, message string, pairs ...session.LogPair) {
	w.inner.LogFlowError(ctx, sessionID, stepID, message, sessionFlowPairs(pairs)...)
}

func sessionFlowPairs(pairs []session.LogPair) []biz.LogPair {
	out := make([]biz.LogPair, 0, len(pairs))
	for _, p := range pairs {
		out = append(out, biz.LogPair{Key: p.Key, Value: p.Value})
	}
	return out
}

// ProvideMonitorFlowLogWriter adapts the shared biz.FlowLogWriter to the
// monitor package's mirror port (monitor.FlowLogWriter). Returns nil when
// inner is nil (tests), callers must nil-check.
func ProvideMonitorFlowLogWriter(inner biz.FlowLogWriter) monitor.FlowLogWriter {
	if inner == nil {
		return nil
	}
	return monitorFlowLogWriter{inner: inner}
}

type monitorFlowLogWriter struct {
	inner biz.FlowLogWriter
}

func (w monitorFlowLogWriter) LogFlowStart(ctx context.Context, sessionID, stepID, message string, pairs ...monitor.LogPair) {
	w.inner.LogFlowStart(ctx, sessionID, stepID, message, monitorFlowPairs(pairs)...)
}

func (w monitorFlowLogWriter) LogFlowDone(ctx context.Context, sessionID, stepID, message string, pairs ...monitor.LogPair) {
	w.inner.LogFlowDone(ctx, sessionID, stepID, message, monitorFlowPairs(pairs)...)
}

func (w monitorFlowLogWriter) LogFlowError(ctx context.Context, sessionID, stepID, message string, pairs ...monitor.LogPair) {
	w.inner.LogFlowError(ctx, sessionID, stepID, message, monitorFlowPairs(pairs)...)
}

func monitorFlowPairs(pairs []monitor.LogPair) []biz.LogPair {
	out := make([]biz.LogPair, 0, len(pairs))
	for _, p := range pairs {
		out = append(out, biz.LogPair{Key: p.Key, Value: p.Value})
	}
	return out
}

func flowPairs(pairs []biz.LogPair) []event.Pair {
	out := make([]event.Pair, 0, len(pairs))
	for _, p := range pairs {
		out = append(out, event.P(p.Key, p.Value))
	}
	return out
}

// domainForStepID derives the trace domain from the stepID prefix so biz
// callers don't handle event.TraceDomain directly.
func domainForStepID(stepID string) event.TraceDomain {
	switch {
	case strings.HasPrefix(stepID, "chat."):
		return event.TraceDomainChat
	case strings.HasPrefix(stepID, "team."):
		return event.TraceDomainTeam
	case strings.HasPrefix(stepID, "graph."):
		return event.TraceDomainGraph
	case strings.HasPrefix(stepID, "channel."):
		return event.TraceDomainChannel
	case strings.HasPrefix(stepID, "knowledge."):
		return event.TraceDomainKnowledge
	case strings.HasPrefix(stepID, "skill."):
		return event.TraceDomainSkill
	case strings.HasPrefix(stepID, "a2a."):
		return event.TraceDomainA2A
	case strings.HasPrefix(stepID, "client_tool."):
		return event.TraceDomainClientTool
	default:
		return event.TraceDomainSystem
	}
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
