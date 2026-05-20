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

	mu       sync.Mutex
	timers   map[string]time.Time
	turnStart time.Time
	spans    []map[string]any
	active   map[string]time.Time
	spanSeq  int
	rootID   string
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
	e.endSpan(e.rootID, status)
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
	if ev.Response.Usage != nil {
		id := e.startSpan("llm.call", e.rootID, map[string]any{
			"prompt_tokens":     ev.Response.Usage.PromptTokens,
			"completion_tokens": ev.Response.Usage.CompletionTokens,
		})
		e.endSpan(id, "ok")
	}
	for _, choice := range ev.Response.Choices {
		for _, tc := range choice.Message.ToolCalls {
			name := tc.Function.Name
			if name == "" {
				continue
			}
			id := e.startSpan("tool.call", e.rootID, map[string]any{"tool_name": name})
			e.endSpan(id, "ok")
		}
	}
}

// WrapEventsWithTraceEmitter tees framework events into span collection.
func WrapEventsWithTraceEmitter(in <-chan *trpcevent.Event, emitter *TraceEmitter) <-chan *trpcevent.Event {
	if emitter == nil || in == nil {
		return in
	}
	out := make(chan *trpcevent.Event, 64)
	go func() {
		defer close(out)
		for ev := range in {
			emitter.ObserveFrameworkEvent(ev)
			out <- ev
		}
	}()
	return out
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
