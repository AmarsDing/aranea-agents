package event

import (
	"aranea-agents/pkg/appctx"
	"aranea-agents/pkg/safego"

	trpcevent "trpc.group/trpc-go/trpc-agent-go/event"
)

// FrameworkSpanObserver receives agent framework events for OTel projection.
type FrameworkSpanObserver interface {
	ObserveFrameworkEvent(ev *trpcevent.Event)
}

// OtelSpanIDSource exposes OTel span ids for usage waterfall correlation.
type OtelSpanIDSource interface {
	LLMSpanOtelID() string
	ToolSpanOtelID(toolCallID string) string
}

// WrapFrameworkEventsWithOtel tees framework events into usage spans and an optional OTel observer,
// also syncing per-span otel_id onto usage waterfall rows when otelSrc is set.
func WrapFrameworkEventsWithOtel(in <-chan *trpcevent.Event, emitter *TraceEmitter, observer FrameworkSpanObserver, otelSrc OtelSpanIDSource) <-chan *trpcevent.Event {
	if in == nil {
		return in
	}
	if emitter == nil && observer == nil {
		return in
	}
	out := make(chan *trpcevent.Event, 64)
	safego.Go(appctx.Ctx(), "event.framework_events.tee", func() {
		defer close(out)
		for ev := range in {
			if emitter != nil {
				emitter.ObserveFrameworkEvent(ev)
				if otelSrc != nil {
					emitter.SyncOtelSpanIDs(otelSrc)
				}
			}
			if observer != nil {
				observer.ObserveFrameworkEvent(ev)
			}
			out <- ev
		}
	})
	return out
}
