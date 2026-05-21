package event

import trpcevent "trpc.group/trpc-go/trpc-agent-go/event"

// FrameworkSpanObserver receives agent framework events for OTel projection.
type FrameworkSpanObserver interface {
	ObserveFrameworkEvent(ev *trpcevent.Event)
}

// WrapFrameworkEvents tees framework events into usage spans and an optional OTel observer.
func WrapFrameworkEvents(in <-chan *trpcevent.Event, emitter *TraceEmitter, observer FrameworkSpanObserver) <-chan *trpcevent.Event {
	if in == nil {
		return in
	}
	if emitter == nil && observer == nil {
		return in
	}
	out := make(chan *trpcevent.Event, 64)
	go func() {
		defer close(out)
		for ev := range in {
			if emitter != nil {
				emitter.ObserveFrameworkEvent(ev)
			}
			if observer != nil {
				observer.ObserveFrameworkEvent(ev)
			}
			out <- ev
		}
	}()
	return out
}
