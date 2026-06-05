package event

import (
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
