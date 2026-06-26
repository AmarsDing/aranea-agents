package event

import (
	"context"
	"fmt"
	"strings"

	"aranea-agents/internal/event/contract"
	"aranea-agents/pkg/loggateway"

	frameworktracing "trpc.group/trpc-go/trpc-agent-go/event/tracing"
)

// FlowTracker holds FlowContext + optional SpanCollector + optional UsageAggregator.
// It keeps LogStart/LogDone/LogError etc. method signatures.
type FlowTracker struct {
	infra *Infra
	tc    TraceContext
	fc    *frameworktracing.FlowContext
	sc    *SpanCollector
	ua    *UsageAggregator
	lg    loggateway.Logger
	// rootSpanID is the OTel turn-root span ID, populated via SetOtelRefs.
	// Embedded into each FlowLogEntry.SpanID to align FlowLog with OTel trace.
	rootSpanID string
}

// NewFlowTracker creates a FlowTracker with injected Infra (replaces bus parameter).
func NewFlowTracker(infra *Infra, tc TraceContext, lg loggateway.Logger) *FlowTracker {
	fc := frameworktracing.NewFlowContext()
	uc := frameworktracing.NewUsageContext()
	ft := &FlowTracker{
		infra: infra,
		tc:    tc,
		fc:    fc,
	}
	sc := NewSpanCollector(frameworktracing.NewSpanContext(), uc, tc)
	ft.sc = sc
	ft.ua = NewUsageAggregator(sc, uc, tc)
	return ft
}

// SpanCollector returns the optional SpanCollector.
func (ft *FlowTracker) SpanCollector() *SpanCollector { return ft.sc }

// UsageAggregator returns the optional UsageAggregator.
func (ft *FlowTracker) UsageAggregator() *UsageAggregator { return ft.ua }

// FlowContextState returns the flow context state.
func (ft *FlowTracker) FlowContextState() *frameworktracing.FlowContext { return ft.fc }

func (ft *FlowTracker) LogStart(stepID, message string, extra ...Pair) {
	ft.fc.RecordStart(stepID)
	ft.emit(stepID, FlowPhaseStart, "", message, "", nil, extra)
}

func (ft *FlowTracker) LogDone(stepID, message string, extra ...Pair) {
	timing := ft.fc.TakeTiming(stepID)
	ft.emit(stepID, FlowPhaseDone, "", message, "", timing, extra)
}

func (ft *FlowTracker) LogSkip(stepID, message string, extra ...Pair) {
	ft.emit(stepID, FlowPhaseSkip, FlowSeverityWarn, message, "", nil, extra)
}

func (ft *FlowTracker) LogWarn(stepID, title, message string, extra ...Pair) {
	ft.emit(stepID, FlowPhaseDone, FlowSeverityWarn, message, title, nil, extra)
}

func (ft *FlowTracker) LogError(stepID, message string, extra ...Pair) {
	timing := ft.fc.TakeTiming(stepID)
	ft.emit(stepID, FlowPhaseError, FlowSeverityError, message, "", timing, extra)
}

func (ft *FlowTracker) LogCritical(stepID, message string, extra ...Pair) {
	timing := ft.fc.TakeTiming(stepID)
	ft.emit(stepID, FlowPhaseError, FlowSeverityCritical, message, "", timing, extra)
}

func (ft *FlowTracker) Log(stepID string, phase FlowPhase, message string, extra ...Pair) {
	ft.emit(stepID, phase, "", message, "", nil, extra)
}

func (ft *FlowTracker) FinishRoot(status string) {
	if ft == nil {
		return
	}
	ft.sc.FinishRoot(status)
}

// SetOtelRefs stores OTel trace/span ids for usage metadata correlation
// and FlowLog span_id alignment (Phase 1 of Problem 4: flow_id 碎片化).
// The rootSpanID is embedded into each emitted FlowLogEntry.SpanID, enabling
// cross-reference between FlowLog and OTel trace (Jaeger) via a shared
// span_id. Callers: service/chat_orchestrator_turn.go:278,
// team/runner_team_trpc_phases.go:103 — both pass bridge.RootSpanID().
func (ft *FlowTracker) SetOtelRefs(traceID, rootSpanID string) {
	if ft == nil {
		return
	}
	ft.rootSpanID = rootSpanID
	ft.ua.SetOtelRefs(traceID, rootSpanID)
}

// CompleteToolCall ends a tool.call usage span using hook-measured duration.
func (ft *FlowTracker) CompleteToolCall(toolCallID, toolName string, durationMS int, status string) {
	if ft == nil || toolCallID == "" {
		return
	}
	ft.sc.CompleteToolCall(toolCallID, toolName, durationMS, status)
}

// TraceID returns the correlation trace id for this run.
func (ft *FlowTracker) TraceID() string {
	if ft == nil {
		return ""
	}
	return strings.TrimSpace(ft.tc.TraceID)
}

// RunID returns the run id for this emitter.
func (ft *FlowTracker) RunID() string {
	if ft == nil {
		return ""
	}
	return strings.TrimSpace(ft.tc.RunID)
}

// SyncOtelSpanIDs copies OTel child span ids onto usage waterfall rows.
func (ft *FlowTracker) SyncOtelSpanIDs(src OtelSpanIDSource) {
	if ft == nil || src == nil {
		return
	}
	ft.sc.SyncOtelSpanIDs(src)
}

// MetadataJSON returns spans + trace_id for usage.metadata_json.
func (ft *FlowTracker) MetadataJSON() string {
	if ft == nil {
		return "{}"
	}
	return ft.sc.MetadataJSON()
}

func (ft *FlowTracker) emit(stepID string, phase FlowPhase, explicitSev FlowSeverity, message, titleOverride string, timing *frameworktracing.FlowTiming, extra []Pair) {
	if ft == nil {
		return
	}
	ex := make(map[string]any, len(extra))
	for _, p := range extra {
		ex[p.Key] = p.Value
	}
	var flowErr *FlowError
	if phase == FlowPhaseError && message != "" {
		flowErr = &FlowError{Message: message}
		if errVal, ok := ex["error"]; ok {
			flowErr.Message = fmt.Sprint(errVal)
		}
	}
	// Convert framework FlowTiming to project FlowTiming (adds StartedAt field).
	var projectTiming *FlowTiming
	if timing != nil {
		projectTiming = &FlowTiming{DurationMS: timing.DurationMS}
	}
	entry := newFlowLogEntry(ft.tc, ft.rootSpanID, stepID, phase, explicitSev, titleOverride, message, "", projectTiming, flowErr, ex)

	if ft.lg != nil {
		ft.lg.Info(entry.displayText(),
			loggateway.StepID(stepID),
			loggateway.Str("phase", string(phase)),
			loggateway.SessionID(ft.tc.SessionID),
		)
	}

	if ft.infra == nil {
		return
	}
	if ft.infra.MonitorEventBus != nil {
		ev := contract.NewMonitorEvent(contract.MonitorEventTypeFlowLog, "flow")
		ev.SessionID = ft.tc.SessionID
		ev.Message = entry.displayText()
		ev.Metadata = entry.toMetadata()
		ft.infra.MonitorEventBus.Publish(context.Background(), ev)
	}
}
