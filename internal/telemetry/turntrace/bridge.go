// Package turntrace provides OTel turn-level spans shared by chat, team, and graph paths.
package turntrace

import (
	"context"
	"sync"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"aranea-agents/internal/event"

	trpcevent "trpc.group/trpc-go/trpc-agent-go/event"
)

// Domain identifies the turn span namespace.
type Domain string

const (
	DomainChat  Domain = "chat"
	DomainTeam  Domain = "team"
	DomainGraph Domain = "graph"
)

const tracerPrefix = "aranea-agents/"

// Orchestration phase names used by StartPhase/EndPhase.
const (
	PhasePlan  = "plan"  // Spirit 规划阶段
	PhaseAlloc = "alloc" // Spirit 分配阶段
	PhaseOrch  = "orch"  // Spirit 编排阶段
)

// Config configures a turn root span.
type Config struct {
	Domain    Domain
	SpanName  string
	SessionID string
	RunID     string
	AgentKey  string
	Extra     []attribute.KeyValue
}

// Bridge manages OTel spans for one turn/run.
type Bridge struct {
	mu     sync.Mutex
	domain Domain
	root   trace.Span
	llm    trace.Span
	tool   map[string]trace.Span
	// Orchestration phase spans (P3-2): plan/alloc/orch are children of root.
	plan     trace.Span
	alloc    trace.Span
	orch     trace.Span
	finished bool
}

type bridgeKey struct{}

// WithBridge attaches a Bridge to ctx for downstream hooks (tools, async graph consume).
func WithBridge(ctx context.Context, b *Bridge) context.Context {
	if ctx == nil || b == nil {
		return ctx
	}
	return context.WithValue(ctx, bridgeKey{}, b)
}

// FromContext returns the Bridge stored in ctx, if any.
func FromContext(ctx context.Context) *Bridge {
	if ctx == nil {
		return nil
	}
	b, _ := ctx.Value(bridgeKey{}).(*Bridge)
	return b
}

// Start opens the turn root span and returns an updated ctx.
func Start(ctx context.Context, cfg Config) (context.Context, *Bridge, trace.Span) {
	name := cfg.SpanName
	if name == "" {
		name = string(cfg.Domain) + ".turn"
	}
	tracer := otel.Tracer(tracerPrefix + string(cfg.Domain))
	attrs := []attribute.KeyValue{
		attribute.String("session_id", cfg.SessionID),
		attribute.String("run_id", cfg.RunID),
	}
	if cfg.AgentKey != "" {
		attrs = append(attrs, attribute.String("agent_key", cfg.AgentKey))
	}
	attrs = append(attrs, cfg.Extra...)
	ctx, span := tracer.Start(ctx, name, trace.WithAttributes(attrs...))
	domain := cfg.Domain
	if domain == "" {
		domain = DomainChat
	}
	return ctx, &Bridge{domain: domain, root: span, tool: make(map[string]trace.Span)}, span
}

// Finish ends the root span and any still-open child spans.
func (b *Bridge) Finish(err error) {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.finished {
		return
	}
	b.finished = true
	if b.llm != nil {
		endSpan(b.llm, err)
		b.llm = nil
	}
	for id, sp := range b.tool {
		endSpan(sp, err)
		delete(b.tool, id)
	}
	// Close any open orchestration phase spans (P3-2).
	endSpan(b.orch, err)
	b.orch = nil
	endSpan(b.alloc, err)
	b.alloc = nil
	endSpan(b.plan, err)
	b.plan = nil
	endSpan(b.root, err)
}

// StartChild begins a named child span under the turn root.
func (b *Bridge) StartChild(ctx context.Context, name string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	if b == nil || b.root == nil {
		return ctx, nil
	}
	parentCtx := trace.ContextWithSpan(ctx, b.root)
	return b.tracer().Start(parentCtx, name, trace.WithAttributes(attrs...))
}

// EndChild ends a child span.
func EndChild(span trace.Span, err error) {
	endSpan(span, err)
}

// StartPhase begins a named orchestration phase span (plan/alloc/orch).
// The span is a child of the root span. Returns the span and an updated ctx.
// Nil-safe: a nil Bridge returns (ctx, nil).
func (b *Bridge) StartPhase(ctx context.Context, phase string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	if b == nil || b.root == nil {
		return ctx, nil
	}
	b.mu.Lock()
	if b.finished {
		b.mu.Unlock()
		return ctx, nil
	}
	b.mu.Unlock()

	parentCtx := trace.ContextWithSpan(ctx, b.root)
	newCtx, span := b.tracer().Start(parentCtx, phase, trace.WithAttributes(attrs...))

	b.mu.Lock()
	defer b.mu.Unlock()
	if b.finished {
		endSpan(span, nil)
		return ctx, nil
	}
	switch phase {
	case PhasePlan:
		b.plan = span
	case PhaseAlloc:
		b.alloc = span
	case PhaseOrch:
		b.orch = span
	}
	return newCtx, span
}

// EndPhase ends a phase span by name. Nil-safe and safe for non-started phases.
func (b *Bridge) EndPhase(phase string, err error) {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.finished {
		return
	}
	var sp trace.Span
	switch phase {
	case PhasePlan:
		sp = b.plan
		b.plan = nil
	case PhaseAlloc:
		sp = b.alloc
		b.alloc = nil
	case PhaseOrch:
		sp = b.orch
		b.orch = nil
	}
	endSpan(sp, err)
}

// ObserveFrameworkEvent opens or updates OTel spans for LLM usage and tool calls.
// Tool spans are closed only via RecordToolCallEnd (hook path).
func (b *Bridge) ObserveFrameworkEvent(ev *trpcevent.Event) {
	if b == nil || ev == nil || ev.Response == nil {
		return
	}
	rsp := ev.Response
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.finished {
		return
	}
	if rsp.Usage != nil {
		if b.llm == nil {
			ctx := trace.ContextWithSpan(context.Background(), b.root)
			_, b.llm = b.tracer().Start(ctx, "llm.call",
				trace.WithAttributes(
					attribute.Int("prompt_tokens", rsp.Usage.PromptTokens),
					attribute.Int("completion_tokens", rsp.Usage.CompletionTokens),
				))
		} else {
			b.llm.SetAttributes(
				attribute.Int("prompt_tokens", rsp.Usage.PromptTokens),
				attribute.Int("completion_tokens", rsp.Usage.CompletionTokens),
			)
		}
	}
	if rsp.IsToolCallResponse() {
		for _, id := range rsp.GetToolCallIDs() {
			if id == "" || b.tool[id] != nil {
				continue
			}
			name := event.ToolNameFromResponse(rsp, id)
			ctx := trace.ContextWithSpan(context.Background(), b.root)
			_, sp := b.tracer().Start(ctx, "tool.call",
				trace.WithAttributes(
					attribute.String("tool_call_id", id),
					attribute.String("tool_name", name),
				))
			b.tool[id] = sp
		}
	}
}

// RecordToolCallEnd closes an OTel tool span using hook-measured duration (sole close path).
func (b *Bridge) RecordToolCallEnd(toolCallID, toolName string, err error) {
	if b == nil || toolCallID == "" {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.finished {
		return
	}
	if sp, ok := b.tool[toolCallID]; ok {
		if toolName != "" {
			sp.SetAttributes(attribute.String("tool_name", toolName))
		}
		endSpan(sp, err)
		delete(b.tool, toolCallID)
		return
	}
	ctx := trace.ContextWithSpan(context.Background(), b.root)
	_, sp := b.tracer().Start(ctx, "tool.call",
		trace.WithAttributes(
			attribute.String("tool_call_id", toolCallID),
			attribute.String("tool_name", toolName),
		))
	endSpan(sp, err)
}

// RootSpanID returns the OTel span id of the turn root.
func (b *Bridge) RootSpanID() string {
	if b == nil || b.root == nil {
		return ""
	}
	return b.root.SpanContext().SpanID().String()
}

// TraceID returns the OTel trace id when valid.
func (b *Bridge) TraceID() string {
	if b == nil || b.root == nil {
		return ""
	}
	sc := b.root.SpanContext()
	if !sc.IsValid() {
		return ""
	}
	return sc.TraceID().String()
}

// LLMSpanOtelID returns the OTel span id for the active llm.call child span.
func (b *Bridge) LLMSpanOtelID() string {
	if b == nil || b.llm == nil {
		return ""
	}
	return b.llm.SpanContext().SpanID().String()
}

// ToolSpanOtelID returns the OTel span id for a tool call child span.
func (b *Bridge) ToolSpanOtelID(toolCallID string) string {
	if b == nil || toolCallID == "" {
		return ""
	}
	b.mu.Lock()
	sp := b.tool[toolCallID]
	b.mu.Unlock()
	if sp == nil {
		return ""
	}
	return sp.SpanContext().SpanID().String()
}

func (b *Bridge) tracer() trace.Tracer {
	if b == nil || b.domain == "" {
		return otel.Tracer(tracerPrefix + string(DomainChat))
	}
	return otel.Tracer(tracerPrefix + string(b.domain))
}

func endSpan(span trace.Span, err error) {
	if span == nil {
		return
	}
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	} else {
		span.SetStatus(codes.Ok, "")
	}
	span.End()
}
