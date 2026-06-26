package event

import (
	"aranea-agents/pkg/loggateway"

	trpcevent "trpc.group/trpc-go/trpc-agent-go/event"
)

// TraceEmitter is the v2 unified writer: FlowLog (WS) + span buffer (usage metadata).
//
// It embeds *FlowTracker for method promotion (composition, NOT inheritance):
// FlowTracker's exported methods (LogStart/LogDone/LogError/...) are surfaced
// on TraceEmitter so callers can use a single value for both flow logging
// and framework event observation. TraceEmitter does not override any
// FlowTracker method; it only adds ObserveFrameworkEvent.
//
// Embedding is preferred over a forwarding wrapper here because every
// FlowTracker method is part of the public flow-log API and would otherwise
// need to be manually re-declared. See go-oop-review §3.2 for the
// "embedding for composition" pattern.
type TraceEmitter struct {
	*FlowTracker
}

// NewTraceEmitter creates an emitter and opens the root chat.turn span.
// The bus parameter is accepted for backward compatibility; it is wrapped into a minimal Infra.
func NewTraceEmitter(bus Bus, tc TraceContext, lg loggateway.Logger) *TraceEmitter {
	var infra *Infra
	if bus != nil {
		infra = &Infra{
			SessionBus: bus,
			MonitorBus: bus,
		}
	}
	ft := NewFlowTracker(infra, tc, lg)
	return &TraceEmitter{FlowTracker: ft}
}

// ObserveFrameworkEvent records llm/tool spans from the agent event stream.
func (e *TraceEmitter) ObserveFrameworkEvent(ev *trpcevent.Event) {
	if e == nil || ev == nil || ev.Response == nil {
		return
	}
	e.FlowTracker.UsageAggregator().ObserveFrameworkEvent(ev)
}
