package event

import "context"

type traceEmitterKey struct{}

// WithTraceEmitter attaches a TraceEmitter to ctx.
func WithTraceEmitter(ctx context.Context, e *TraceEmitter) context.Context {
	if ctx == nil || e == nil {
		return ctx
	}
	return context.WithValue(ctx, traceEmitterKey{}, e)
}

// TraceEmitterFromContext returns the TraceEmitter stored in ctx, if any.
func TraceEmitterFromContext(ctx context.Context) *TraceEmitter {
	if ctx == nil {
		return nil
	}
	em, _ := ctx.Value(traceEmitterKey{}).(*TraceEmitter)
	return em
}

// WithFlowLogger is an alias for WithTraceEmitter (v2).
func WithFlowLogger(ctx context.Context, e *TraceEmitter) context.Context {
	return WithTraceEmitter(ctx, e)
}

// FlowLoggerFromContext is an alias for TraceEmitterFromContext (v2).
func FlowLoggerFromContext(ctx context.Context) *TraceEmitter {
	return TraceEmitterFromContext(ctx)
}

func FlowLogError(bus Bus, buffer *Buffer, sessionID, agentKey, step, msg string, extra ...Pair) {
	tc := TraceContext{SessionID: sessionID, AgentKey: agentKey, Domain: TraceDomainChat}
	if tc.TraceID == "" {
		tc = NewTraceContext(context.Background(), TraceOpts{SessionID: sessionID, AgentKey: agentKey, Domain: TraceDomainChat})
	}
	NewTraceEmitter(bus, buffer, tc).LogError(step, msg, extra...)
}

func FlowLogSkip(bus Bus, buffer *Buffer, sessionID, agentKey, step, msg string, extra ...Pair) {
	tc := NewTraceContext(context.Background(), TraceOpts{SessionID: sessionID, AgentKey: agentKey, Domain: TraceDomainChat})
	NewTraceEmitter(bus, buffer, tc).LogSkip(step, msg, extra...)
}

func FlowLogDone(bus Bus, buffer *Buffer, sessionID, agentKey, step, msg string, extra ...Pair) {
	tc := NewTraceContext(context.Background(), TraceOpts{SessionID: sessionID, AgentKey: agentKey, Domain: TraceDomainChat})
	NewTraceEmitter(bus, buffer, tc).LogDone(step, msg, extra...)
}

func CtxFlowLogError(ctx context.Context, step, msg string, extra ...Pair) {
	if e := TraceEmitterFromContext(ctx); e != nil {
		e.LogError(step, msg, extra...)
		return
	}
	NewTraceEmitter(nil, nil, TraceContext{}).LogError(step, msg, extra...)
}

func CtxFlowLogSkip(ctx context.Context, step, msg string, extra ...Pair) {
	if e := TraceEmitterFromContext(ctx); e != nil {
		e.LogSkip(step, msg, extra...)
		return
	}
	NewTraceEmitter(nil, nil, TraceContext{}).LogSkip(step, msg, extra...)
}

func CtxFlowLogDone(ctx context.Context, step, msg string, extra ...Pair) {
	if e := TraceEmitterFromContext(ctx); e != nil {
		e.LogDone(step, msg, extra...)
		return
	}
	NewTraceEmitter(nil, nil, TraceContext{}).LogDone(step, msg, extra...)
}

func CtxFlowLogWarn(ctx context.Context, step, msg string, extra ...Pair) {
	if e := TraceEmitterFromContext(ctx); e != nil {
		e.LogWarn(step, "", msg, extra...)
		return
	}
	emitSystem(ctx, "", "", step, FlowPhaseDone, FlowSeverityWarn, msg, extra)
}

// NewFlowLogger creates a v2 TraceEmitter (name kept for call-site stability).
func NewFlowLogger(bus Bus, buffer *Buffer, sessionID, agentKey string) *TraceEmitter {
	tc := NewTraceContext(context.Background(), TraceOpts{
		SessionID: sessionID,
		AgentKey:  agentKey,
		Domain:    TraceDomainChat,
	})
	return NewTraceEmitter(bus, buffer, tc)
}

// NewTraceEmitterForRun creates an emitter with full trace context for a chat turn.
func NewTraceEmitterForRun(ctx context.Context, bus Bus, buffer *Buffer, sessionID, runID, agentKey, agentID string) *TraceEmitter {
	tc := NewTraceContext(ctx, TraceOpts{
		SessionID: sessionID,
		RunID:     runID,
		Domain:    TraceDomainChat,
		AgentKey:  agentKey,
		AgentID:   agentID,
	})
	return NewTraceEmitter(bus, buffer, tc)
}
