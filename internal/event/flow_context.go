package event

import (
	"context"

	"aranea-agents/pkg/loggateway"
)

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
//
// Deprecated: Use loggateway.Logger + With() for structured logging.
func WithFlowLogger(ctx context.Context, e *TraceEmitter) context.Context {
	return WithTraceEmitter(ctx, e)
}

// FlowLoggerFromContext is an alias for TraceEmitterFromContext (v2).
//
// Deprecated: Use loggateway.Logger + With() for structured logging.
func FlowLoggerFromContext(ctx context.Context) *TraceEmitter {
	return TraceEmitterFromContext(ctx)
}

// NewFlowLogger creates a v2 TraceEmitter (name kept for call-site stability).
//
// Deprecated: Use loggateway.Logger + With() for structured logging.
func NewFlowLogger(sessionID, agentKey string, lg loggateway.Logger, infra *Infra) *TraceEmitter {
	tc := NewTraceContext(context.Background(), TraceOpts{
		SessionID: sessionID,
		AgentKey:  agentKey,
		Domain:    TraceDomainChat,
	})
	return NewTraceEmitter(infra, tc, lg)
}

// TraceEmitterOpts configures a run-scoped TraceEmitter.
type TraceEmitterOpts struct {
	Ctx       context.Context
	SessionID string
	RunID     string
	AgentKey  string
	AgentID   string
	Domain    TraceDomain
	LG        loggateway.Logger
	// Infra carries the shared MonitorEventBus so flow_log events are
	// published to the monitor pipeline. Nil disables bus publication.
	Infra *Infra
}

// NewTraceEmitterForRun creates an emitter with full trace context for a run.
func NewTraceEmitterForRun(opts TraceEmitterOpts) *TraceEmitter {
	domain := opts.Domain
	if domain == "" {
		domain = TraceDomainChat
	}
	tc := NewTraceContext(opts.Ctx, TraceOpts{
		SessionID: opts.SessionID,
		RunID:     opts.RunID,
		Domain:    domain,
		AgentKey:  opts.AgentKey,
		AgentID:   opts.AgentID,
	})
	return NewTraceEmitter(opts.Infra, tc, opts.LG)
}
