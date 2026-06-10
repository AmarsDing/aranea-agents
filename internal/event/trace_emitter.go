package event

import (
	"context"

	"aranea-agents/pkg/loggateway"

	trpcevent "trpc.group/trpc-go/trpc-agent-go/event"
)

// TraceEmitter is the v2 unified writer: FlowLog (WS) + span buffer (usage metadata).
// It embeds FlowTracker and adds ObserveFrameworkEvent for trpc-agent-go event stream.
type TraceEmitter struct {
	*FlowTracker
}

// NewTraceEmitter creates an emitter and opens the root chat.turn span.
// The bus parameter is accepted for backward compatibility; it is wrapped into a minimal Infra.
func NewTraceEmitter(bus Bus, buffer *Buffer, tc TraceContext, lg loggateway.Logger) *TraceEmitter {
	var infra *Infra
	if bus != nil {
		infra = &Infra{
			SessionBus: bus,
			MonitorBus: bus,
			Buffer:     buffer,
		}
	}
	ft := NewFlowTracker(infra, buffer, tc, lg)
	return &TraceEmitter{FlowTracker: ft}
}

// ObserveFrameworkEvent records llm/tool spans from the agent event stream.
func (e *TraceEmitter) ObserveFrameworkEvent(ev *trpcevent.Event) {
	if e == nil || ev == nil || ev.Response == nil {
		return
	}
	e.FlowTracker.UsageAggregator().ObserveFrameworkEvent(ev)
}

// EmitProgress publishes a chat-visible execution progress envelope.
// It is independent from flow_log (which still goes to monitor bus) and is intended
// for the AgentTreeTimeline to render an inline progress card during the long
// 5-15s wait for LLM first byte.
//
// phase must be one of "start" | "done" | "error":
//   - "start" records a flow timing so "done" can compute duration_ms
//   - "done"  publishes the envelope with duration_ms from the recorded timing
//   - "error" publishes with the error message attached
//
// category is a UI hint: "orchestration" | "team" | "tool" | "thinking".
//
// ctx is required so the publish inherits caller cancellation. Passing
// context.Background() at call sites defeats the cancellation chain
// (BC5 — long-running goroutines should be ctx-cancellable).
//
// See docs/reports/2026-06-10-proposal-execution-progress-inline.md
func (e *TraceEmitter) EmitProgress(ctx context.Context, stepID, phase, message, category string, extra ...Pair) {
	if e == nil || e.infra == nil || e.infra.SessionBus == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	stepID = normalizeStepID(stepID)
	if phase == "start" && e.fc != nil {
		e.fc.RecordStart(stepID)
	}
	env := NewEnvelope(EnvelopeTypeExecutionProgress, "system", e.tc.SessionID)
	env.Channel = "chat"
	env.Metadata = map[string]any{
		"step_id":  stepID,
		"phase":    phase,
		"message":  message,
		"category": category,
	}
	if phase == "done" && e.fc != nil {
		if timing := e.fc.TakeTiming(stepID); timing != nil {
			env.Metadata["duration_ms"] = timing.DurationMS
		}
	}
	for _, p := range extra {
		env.Metadata[p.Key] = p.Value
	}
	e.infra.SessionBus.Publish(ctx, env)
}
