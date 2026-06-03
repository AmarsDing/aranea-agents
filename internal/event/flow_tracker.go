package event

import (
	"context"
	"fmt"
	"os"
	"strings"
)

// FlowTracker holds FlowContext + optional SpanCollector + optional UsageAggregator.
// It keeps LogStart/LogDone/LogError etc. method signatures.
type FlowTracker struct {
	infra  *Infra
	buffer *Buffer
	tc     TraceContext
	fc     *FlowContext
	sc     *SpanCollector
	ua     *UsageAggregator
}

// NewFlowTracker creates a FlowTracker with injected Infra (replaces bus parameter).
func NewFlowTracker(infra *Infra, buffer *Buffer, tc TraceContext) *FlowTracker {
	fc := NewFlowContext()
	uc := NewUsageContext()
	ft := &FlowTracker{
		infra:  infra,
		buffer: buffer,
		tc:     tc,
		fc:     fc,
	}
	sc := NewSpanCollector(NewSpanContext(), uc, tc)
	ft.sc = sc
	ft.ua = NewUsageAggregator(sc, uc, tc)
	return ft
}

// SpanCollector returns the optional SpanCollector.
func (ft *FlowTracker) SpanCollector() *SpanCollector { return ft.sc }

// UsageAggregator returns the optional UsageAggregator.
func (ft *FlowTracker) UsageAggregator() *UsageAggregator { return ft.ua }

// FlowContextState returns the flow context state.
func (ft *FlowTracker) FlowContextState() *FlowContext { return ft.fc }

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
	if ft.shouldPublishFlowChatError(stepID) {
		var bus Bus
		if ft.infra != nil {
			bus = ft.infra.SessionBus
		}
		if bus != nil {
			errEnv := NewEnvelope(EnvelopeTypeError, "flow", ft.tc.SessionID)
			errEnv.Error = &EnvelopeError{
				Type:    "flow_" + normalizeStepID(stepID),
				Message: message,
			}
			bus.Publish(context.Background(), errEnv)
		}
	}
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

// SetOtelRefs stores OTel trace/span ids for usage metadata correlation.
func (ft *FlowTracker) SetOtelRefs(traceID, rootSpanID string) {
	if ft == nil {
		return
	}
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

// flowStepsSkipChatError lists monitor-only flow errors that must not surface as chat toasts.
var flowStepsSkipChatError = map[string]struct{}{
	"chat.usage_record":       {},
	"system.agent.tool_build": {},
}

// shouldPublishFlowChatError checks if a flow error should surface as a chat toast.
func (ft *FlowTracker) shouldPublishFlowChatError(stepID string) bool {
	_, skip := flowStepsSkipChatError[normalizeStepID(stepID)]
	return !skip
}

func (ft *FlowTracker) emit(stepID string, phase FlowPhase, explicitSev FlowSeverity, message, titleOverride string, timing *FlowTiming, extra []Pair) {
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
	entry := newFlowLogEntry(ft.tc, stepID, phase, explicitSev, titleOverride, message, "", timing, flowErr, ex)

	if os.Getenv("FLOW_LOG_STDERR") == "1" {
		fmt.Fprintf(os.Stderr, "[flow] %s\n", entry.displayText())
		_ = os.Stderr.Sync()
	}

	if ft.infra == nil {
		return
	}
	env := NewEnvelope(EnvelopeTypeFlowLog, "flow", ft.tc.SessionID)
	env.Channel = "monitor"
	env.Content = &EnvelopeContent{
		Text:      entry.displayText(),
		IsPartial: false,
	}
	env.Metadata = entry.toMetadata()
	if ft.buffer != nil {
		ft.buffer.Append(env)
	}
	ft.infra.Publish(context.Background(), env)
}
