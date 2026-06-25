package event

import (
	trpcevent "trpc.group/trpc-go/trpc-agent-go/event"

	frameworktracing "trpc.group/trpc-go/trpc-agent-go/event/tracing"
)

// UsageAggregator observes framework events and aggregates usage metadata.
type UsageAggregator struct {
	sc *SpanCollector
	uc *frameworktracing.UsageContext
	tc TraceContext
}

// NewUsageAggregator creates a UsageAggregator.
func NewUsageAggregator(sc *SpanCollector, uc *frameworktracing.UsageContext, tc TraceContext) *UsageAggregator {
	return &UsageAggregator{sc: sc, uc: uc, tc: tc}
}

// ObserveFrameworkEvent records llm/tool spans from the agent event stream.
func (a *UsageAggregator) ObserveFrameworkEvent(ev *trpcevent.Event) {
	if a == nil || ev == nil || ev.Response == nil {
		return
	}
	rsp := ev.Response
	if rsp.Usage != nil {
		a.sc.StartLLMSpan(rsp.Usage.PromptTokens, rsp.Usage.CompletionTokens)
	}
	if rsp.IsToolCallResponse() {
		for _, id := range rsp.GetToolCallIDs() {
			if id == "" {
				continue
			}
			if a.sc.HasToolSpan(id) {
				continue
			}
			name := ToolNameFromResponse(rsp, id)
			a.sc.OpenToolSpan(name, id)
		}
	}
}

// SetOtelRefs stores OTel trace/span ids for usage metadata correlation.
func (a *UsageAggregator) SetOtelRefs(traceID, rootSpanID string) {
	if a == nil {
		return
	}
	a.uc.SetOtelRefs(traceID, rootSpanID)
}

// SyncOtelSpanIDs delegates to SpanCollector.
func (a *UsageAggregator) SyncOtelSpanIDs(src OtelSpanIDSource) {
	if a == nil {
		return
	}
	a.sc.SyncOtelSpanIDs(src)
}

// MetadataJSON delegates to SpanCollector.
func (a *UsageAggregator) MetadataJSON() string {
	if a == nil {
		return "{}"
	}
	return a.sc.MetadataJSON()
}
