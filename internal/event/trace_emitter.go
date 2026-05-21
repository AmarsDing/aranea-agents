package event

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"aranea-agents/pkg/safego"

	trpcevent "trpc.group/trpc-go/trpc-agent-go/event"
)

// TraceEmitter is the v2 unified writer: FlowLog (WS) + span buffer (usage metadata).
type TraceEmitter struct {
	bus    Bus
	buffer *Buffer
	tc     TraceContext

	mu          sync.Mutex
	timers      map[string]time.Time
	turnStart   time.Time
	spans       []map[string]any
	active      map[string]time.Time
	openTools   map[string]string // tool_call_id -> usage span id
	openLLMSpan string
	spanSeq     int
	rootID      string
	otelTraceID string
	otelRootID  string
}

// NewTraceEmitter creates an emitter and opens the root chat.turn span.
func NewTraceEmitter(bus Bus, buffer *Buffer, tc TraceContext) *TraceEmitter {
	e := &TraceEmitter{
		bus:       bus,
		buffer:    buffer,
		tc:        tc,
		timers:    make(map[string]time.Time),
		turnStart: time.Now(),
		active:    make(map[string]time.Time),
		openTools: make(map[string]string),
	}
	e.rootID = e.startSpan("chat.turn", "", map[string]any{
		"trace_id":  tc.TraceID,
		"run_id":    tc.RunID,
		"agent_key": tc.AgentKey,
	})
	return e
}

func (e *TraceEmitter) LogStart(stepID, message string, extra ...Pair) {
	e.mu.Lock()
	e.timers[stepID] = time.Now()
	e.mu.Unlock()
	e.emit(stepID, FlowPhaseStart, "", message, "", nil, extra)
}

func (e *TraceEmitter) LogDone(stepID, message string, extra ...Pair) {
	var timing *FlowTiming
	e.mu.Lock()
	if start, ok := e.timers[stepID]; ok {
		timing = &FlowTiming{DurationMS: time.Since(start).Milliseconds()}
		delete(e.timers, stepID)
	}
	e.mu.Unlock()
	e.emit(stepID, FlowPhaseDone, "", message, "", timing, extra)
}

func (e *TraceEmitter) LogSkip(stepID, message string, extra ...Pair) {
	e.emit(stepID, FlowPhaseSkip, FlowSeverityWarn, message, "", nil, extra)
}

func (e *TraceEmitter) LogWarn(stepID, title, message string, extra ...Pair) {
	e.emit(stepID, FlowPhaseDone, FlowSeverityWarn, message, title, nil, extra)
}

func (e *TraceEmitter) LogError(stepID, message string, extra ...Pair) {
	var timing *FlowTiming
	e.mu.Lock()
	if start, ok := e.timers[stepID]; ok {
		timing = &FlowTiming{DurationMS: time.Since(start).Milliseconds()}
		delete(e.timers, stepID)
	}
	e.mu.Unlock()
	e.emit(stepID, FlowPhaseError, FlowSeverityError, message, "", timing, extra)
	if e.bus != nil && shouldPublishFlowChatError(stepID) {
		errEnv := NewEnvelope(EnvelopeTypeError, "flow", e.tc.SessionID)
		errEnv.Error = &EnvelopeError{
			Type:    "flow_" + normalizeStepID(stepID),
			Message: message,
		}
		bus := e.bus
		safego.Go(context.Background(), "flow-error-publish", func() {
			bus.Publish(context.Background(), errEnv)
		})
	}
}

func (e *TraceEmitter) LogCritical(stepID, message string, extra ...Pair) {
	var timing *FlowTiming
	e.mu.Lock()
	if start, ok := e.timers[stepID]; ok {
		timing = &FlowTiming{DurationMS: time.Since(start).Milliseconds()}
		delete(e.timers, stepID)
	}
	e.mu.Unlock()
	e.emit(stepID, FlowPhaseError, FlowSeverityCritical, message, "", timing, extra)
}

func (e *TraceEmitter) Log(stepID string, phase FlowPhase, message string, extra ...Pair) {
	e.emit(stepID, phase, "", message, "", nil, extra)
}

func (e *TraceEmitter) FinishRoot(status string) {
	if e == nil {
		return
	}
	e.mu.Lock()
	llmID := e.openLLMSpan
	e.openLLMSpan = ""
	pending := make([]string, 0, len(e.openTools))
	for _, spanID := range e.openTools {
		pending = append(pending, spanID)
	}
	e.openTools = make(map[string]string)
	rootID := e.rootID
	e.mu.Unlock()
	if llmID != "" {
		e.endSpan(llmID, status)
	}
	for _, spanID := range pending {
		e.endSpan(spanID, status)
	}
	e.endSpan(rootID, status)
}

// SetOtelRefs stores OTel trace/span ids for usage metadata correlation.
func (e *TraceEmitter) SetOtelRefs(traceID, rootSpanID string) {
	if e == nil {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.otelTraceID = strings.TrimSpace(traceID)
	e.otelRootID = strings.TrimSpace(rootSpanID)
}

// CompleteToolCall ends a tool.call usage span using hook-measured duration.
func (e *TraceEmitter) CompleteToolCall(toolCallID, toolName string, durationMS int, status string) {
	if e == nil || toolCallID == "" {
		return
	}
	e.mu.Lock()
	spanID, ok := e.openTools[toolCallID]
	delete(e.openTools, toolCallID)
	e.mu.Unlock()
	if !ok {
		spanID = e.startSpan("tool.call", e.rootID, map[string]any{
			"tool_name":    toolName,
			"tool_call_id": toolCallID,
		})
	}
	e.endSpanWithDuration(spanID, normalizeSpanStatus(status), durationMS)
}

// TraceID returns the correlation trace id for this run.
func (e *TraceEmitter) TraceID() string {
	if e == nil {
		return ""
	}
	return strings.TrimSpace(e.tc.TraceID)
}

// RunID returns the run id for this emitter.
func (e *TraceEmitter) RunID() string {
	if e == nil {
		return ""
	}
	return strings.TrimSpace(e.tc.RunID)
}

// SyncOtelSpanIDs copies OTel child span ids onto usage waterfall rows (otel_id field).
func (e *TraceEmitter) SyncOtelSpanIDs(src OtelSpanIDSource) {
	if e == nil || src == nil {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	for i := range e.spans {
		name, _ := e.spans[i]["name"].(string)
		switch name {
		case "llm.call":
			if id := src.LLMSpanOtelID(); id != "" {
				e.spans[i]["otel_id"] = id
			}
		case "tool.call":
			tid, _ := e.spans[i]["tool_call_id"].(string)
			if id := src.ToolSpanOtelID(tid); id != "" {
				e.spans[i]["otel_id"] = id
			}
		case "chat.turn":
			if e.otelRootID != "" {
				e.spans[i]["otel_id"] = e.otelRootID
			}
		}
	}
}

// MetadataJSON returns spans + trace_id for usage.metadata_json.
func (e *TraceEmitter) MetadataJSON() string {
	if e == nil {
		return "{}"
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	payload := map[string]any{
		"trace_id":      e.tc.TraceID,
		"spans":         e.spans,
		"trace_root_ms": e.turnStart.UnixMilli(),
	}
	if e.otelTraceID != "" {
		payload["otel_trace_id"] = e.otelTraceID
	}
	if e.otelRootID != "" {
		payload["otel_root_span_id"] = e.otelRootID
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "{}"
	}
	return string(raw)
}

// ObserveFrameworkEvent records llm/tool spans from the agent event stream.
func (e *TraceEmitter) ObserveFrameworkEvent(ev *trpcevent.Event) {
	if e == nil || ev == nil || ev.Response == nil {
		return
	}
	rsp := ev.Response
	if rsp.Usage != nil {
		e.mergeLLMSpan(rsp.Usage.PromptTokens, rsp.Usage.CompletionTokens)
	}
	if rsp.IsToolCallResponse() {
		for _, id := range rsp.GetToolCallIDs() {
			if id == "" {
				continue
			}
			e.mu.Lock()
			if _, exists := e.openTools[id]; exists {
				e.mu.Unlock()
				continue
			}
			e.mu.Unlock()
			name := ToolNameFromResponse(rsp, id)
			spanID := e.startSpan("tool.call", e.rootID, map[string]any{
				"tool_name":    name,
				"tool_call_id": id,
			})
			e.mu.Lock()
			e.openTools[id] = spanID
			e.mu.Unlock()
		}
	}
}

func (e *TraceEmitter) mergeLLMSpan(promptTok, completionTok int) {
	e.mu.Lock()
	if e.openLLMSpan != "" {
		for i := range e.spans {
			if e.spans[i]["id"] == e.openLLMSpan {
				e.spans[i]["prompt_tokens"] = promptTok
				e.spans[i]["completion_tokens"] = completionTok
				e.mu.Unlock()
				return
			}
		}
	}
	e.mu.Unlock()
	id := e.startSpan("llm.call", e.rootID, map[string]any{
		"prompt_tokens":     promptTok,
		"completion_tokens": completionTok,
	})
	e.mu.Lock()
	e.openLLMSpan = id
	e.mu.Unlock()
}

func normalizeSpanStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "", "success", "ok":
		return "ok"
	case "error", "failed":
		return "error"
	default:
		return status
	}
}

func (e *TraceEmitter) emit(stepID string, phase FlowPhase, explicitSev FlowSeverity, message, titleOverride string, timing *FlowTiming, extra []Pair) {
	if e == nil {
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
	entry := newFlowLogEntry(e.tc, stepID, phase, explicitSev, titleOverride, message, "", timing, flowErr, ex)

	if os.Getenv("FLOW_LOG_STDERR") == "1" {
		fmt.Fprintf(os.Stderr, "[flow] %s\n", entry.displayText())
		_ = os.Stderr.Sync()
	}

	if e.bus == nil {
		return
	}
	env := NewEnvelope(EnvelopeTypeFlowLog, "flow", e.tc.SessionID)
	env.Channel = "monitor"
	env.Content = &EnvelopeContent{
		Text:      entry.displayText(),
		IsPartial: false,
	}
	env.Metadata = entry.toMetadata()
	if e.buffer != nil {
		e.buffer.Append(env)
	}
	bus := e.bus
	safego.Go(context.Background(), "flow-log-publish", func() {
		bus.Publish(context.Background(), env)
	})
}

func (e *TraceEmitter) startSpan(name, parentID string, attrs map[string]any) string {
	if e == nil {
		return ""
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.spanSeq++
	id := fmt.Sprintf("%s-%d", name, e.spanSeq)
	if parentID == "" && e.rootID != "" && name != "chat.turn" {
		parentID = e.rootID
	}
	start := time.Now()
	e.active[id] = start
	row := map[string]any{
		"id":          id,
		"name":        name,
		"parent_id":   parentID,
		"status":      "running",
		"start_ms":    start.Sub(e.turnStart).Milliseconds(),
		"duration_ms": int64(0),
		"trace_id":    e.tc.TraceID,
	}
	for k, v := range attrs {
		row[k] = v
	}
	if name == "chat.turn" && e.rootID == "" {
		e.rootID = id
	}
	e.spans = append(e.spans, row)
	return id
}

func (e *TraceEmitter) endSpanWithDuration(id, status string, durationMS int) {
	if e == nil || id == "" {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	start, hadActive := e.active[id]
	delete(e.active, id)
	for i := range e.spans {
		if e.spans[i]["id"] == id {
			if durationMS > 0 {
				e.spans[i]["duration_ms"] = int64(durationMS)
			} else if hadActive {
				e.spans[i]["duration_ms"] = time.Since(start).Milliseconds()
			}
			e.spans[i]["status"] = status
			return
		}
	}
}

func (e *TraceEmitter) endSpan(id, status string) {
	if e == nil || id == "" {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	start, ok := e.active[id]
	if !ok {
		return
	}
	delete(e.active, id)
	for i := range e.spans {
		if e.spans[i]["id"] == id {
			e.spans[i]["duration_ms"] = time.Since(start).Milliseconds()
			e.spans[i]["status"] = status
			return
		}
	}
}

// flowStepsSkipChatError lists monitor-only flow errors that must not surface as chat toasts.
var flowStepsSkipChatError = map[string]struct{}{
	"chat.usage_record":       {},
	"system.agent.tool_build": {},
}

func shouldPublishFlowChatError(stepID string) bool {
	_, skip := flowStepsSkipChatError[normalizeStepID(stepID)]
	return !skip
}
