package event

import (
	"context"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"
)

// TraceDomain identifies the business area for a trace.
type TraceDomain string

const (
	TraceDomainChat      TraceDomain = "chat"
	TraceDomainTeam      TraceDomain = "team"
	TraceDomainGraph     TraceDomain = "graph"
	TraceDomainChannel   TraceDomain = "channel"
	TraceDomainKnowledge TraceDomain = "knowledge"
	TraceDomainPlugin    TraceDomain = "plugin"
	TraceDomainSystem    TraceDomain = "system"
)

// TraceContext correlates flow logs and spans for one request/run.
type TraceContext struct {
	TraceID   string
	SessionID string
	RunID     string
	TeamID    string
	Domain    TraceDomain
	AgentKey  string
	AgentID   string
}

// TraceOpts configures a new TraceContext.
type TraceOpts struct {
	SessionID string
	RunID     string
	TeamID    string
	Domain    TraceDomain
	AgentKey  string
	AgentID   string
}

// NewTraceContext builds a trace context, aligning TraceID with OTel when present.
func NewTraceContext(ctx context.Context, opts TraceOpts) TraceContext {
	tc := TraceContext{
		SessionID: opts.SessionID,
		RunID:     opts.RunID,
		TeamID:    opts.TeamID,
		Domain:    opts.Domain,
		AgentKey:  opts.AgentKey,
		AgentID:   opts.AgentID,
	}
	if sc := trace.SpanFromContext(ctx); sc != nil {
		if scCtx := sc.SpanContext(); scCtx.IsValid() {
			tc.TraceID = scCtx.TraceID().String()
		}
	}
	if tc.TraceID == "" {
		tc.TraceID = "tr_" + uuid.NewString()
	}
	if tc.Domain == "" {
		tc.Domain = TraceDomainChat
	}
	return tc
}

type traceContextKey struct{}

func WithTraceContext(ctx context.Context, tc TraceContext) context.Context {
	if ctx == nil {
		return ctx
	}
	return context.WithValue(ctx, traceContextKey{}, tc)
}

func TraceContextFromContext(ctx context.Context) (TraceContext, bool) {
	if ctx == nil {
		return TraceContext{}, false
	}
	tc, ok := ctx.Value(traceContextKey{}).(TraceContext)
	return tc, ok
}
