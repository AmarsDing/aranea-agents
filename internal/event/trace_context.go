package event

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"
)

// TraceDomain identifies the business area for a trace.
type TraceDomain string

const (
	TraceDomainChat       TraceDomain = "chat"
	TraceDomainTeam       TraceDomain = "team"
	TraceDomainGraph      TraceDomain = "graph"
	TraceDomainChannel    TraceDomain = "channel"
	TraceDomainKnowledge  TraceDomain = "knowledge"
	TraceDomainPlugin     TraceDomain = "plugin"
	TraceDomainSystem     TraceDomain = "system"
	TraceDomainSkill      TraceDomain = "skill"
	TraceDomainA2A        TraceDomain = "a2a"
	TraceDomainVoice      TraceDomain = "voice"
	TraceDomainClientTool  TraceDomain = "client_tool"
	TraceDomainAgentBridge TraceDomain = "agentbridge"
	TraceDomainComputerUse TraceDomain = "computeruse"
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

// traceIDKey carries an explicit trace ID through context. This is the
// noop-proof propagation channel: production runs with the noop OTel
// provider by default (no OTEL_EXPORTER_OTLP_ENDPOINT), so span-context
// inheritance never happens and every emitter would otherwise mint its own
// random trace id, fragmenting one conversation turn into many rows.
type traceIDKey struct{}

// ContextWithTraceID stores an explicit trace ID on ctx. Empty ids are
// ignored so callers can pass through unresolved values safely.
func ContextWithTraceID(ctx context.Context, traceID string) context.Context {
	if ctx == nil || traceID == "" {
		return ctx
	}
	return context.WithValue(ctx, traceIDKey{}, traceID)
}

// TraceIDFromContext returns the explicit trace ID stored on ctx, if any.
func TraceIDFromContext(ctx context.Context) (string, bool) {
	if ctx == nil {
		return "", false
	}
	id, ok := ctx.Value(traceIDKey{}).(string)
	return id, ok && id != ""
}

// GenerateTraceID returns a new OTel-compatible trace id (32 lowercase hex
// chars). Hex format keeps the id valid for OTel remote-parent injection,
// unlike the legacy "tr_" + uuid form which Jaeger rejects.
func GenerateTraceID() string {
	return strings.ReplaceAll(uuid.NewString(), "-", "")
}

// NewTraceContext builds a trace context. TraceID precedence:
//  1. explicit ctx trace id (ContextWithTraceID — survives noop provider)
//  2. valid OTel span context in ctx (real provider inheritance)
//  3. freshly generated hex id
func NewTraceContext(ctx context.Context, opts TraceOpts) TraceContext {
	tc := TraceContext{
		SessionID: opts.SessionID,
		RunID:     opts.RunID,
		TeamID:    opts.TeamID,
		Domain:    opts.Domain,
		AgentKey:  opts.AgentKey,
		AgentID:   opts.AgentID,
	}
	if id, ok := TraceIDFromContext(ctx); ok {
		tc.TraceID = id
	}
	if tc.TraceID == "" {
		if sc := trace.SpanFromContext(ctx); sc != nil {
			if scCtx := sc.SpanContext(); scCtx.IsValid() {
				tc.TraceID = scCtx.TraceID().String()
			}
		}
	}
	if tc.TraceID == "" {
		tc.TraceID = GenerateTraceID()
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
