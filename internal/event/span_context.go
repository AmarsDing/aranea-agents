package event

import (
	"fmt"
	"sync"
	"time"
)

// SpanContext holds span tree data with its own mutex.
type SpanContext struct {
	mu         sync.Mutex
	spans      []map[string]any
	active     map[string]time.Time
	openTools  map[string]string // tool_call_id -> usage span id
	openLLMSpan string
	spanSeq    int
	rootID     string
}

// NewSpanContext creates a SpanContext with initialized maps.
func NewSpanContext() *SpanContext {
	return &SpanContext{
		spans:     make([]map[string]any, 0),
		active:    make(map[string]time.Time),
		openTools: make(map[string]string),
	}
}

// StartSpan adds a new span to the tree and returns its id.
func (sc *SpanContext) StartSpan(name, parentID string, attrs map[string]any, traceID string, turnStart time.Time) string {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	sc.spanSeq++
	id := fmt.Sprintf("%s-%d", name, sc.spanSeq)
	if parentID == "" && sc.rootID != "" && name != "chat.turn" {
		parentID = sc.rootID
	}
	start := time.Now()
	sc.active[id] = start
	row := map[string]any{
		"id":          id,
		"name":        name,
		"parent_id":   parentID,
		"status":      "running",
		"start_ms":    start.Sub(turnStart).Milliseconds(),
		"duration_ms": int64(0),
		"trace_id":    traceID,
	}
	for k, v := range attrs {
		row[k] = v
	}
	if name == "chat.turn" && sc.rootID == "" {
		sc.rootID = id
	}
	sc.spans = append(sc.spans, row)
	return id
}

// EndSpanWithDuration ends a span with explicit duration.
func (sc *SpanContext) EndSpanWithDuration(id, status string, durationMS int) {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	start, hadActive := sc.active[id]
	delete(sc.active, id)
	for i := range sc.spans {
		if sc.spans[i]["id"] == id {
			if durationMS > 0 {
				sc.spans[i]["duration_ms"] = int64(durationMS)
			} else if hadActive {
				sc.spans[i]["duration_ms"] = time.Since(start).Milliseconds()
			}
			sc.spans[i]["status"] = status
			return
		}
	}
}

// EndSpan ends a span using wall-clock duration.
func (sc *SpanContext) EndSpan(id, status string) {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	start, ok := sc.active[id]
	if !ok {
		return
	}
	delete(sc.active, id)
	for i := range sc.spans {
		if sc.spans[i]["id"] == id {
			sc.spans[i]["duration_ms"] = time.Since(start).Milliseconds()
			sc.spans[i]["status"] = status
			return
		}
	}
}

// FinishRoot closes all open spans and the root span.
func (sc *SpanContext) FinishRoot(status string) (llmID string, pending []string, rootID string) {
	sc.mu.Lock()
	llmID = sc.openLLMSpan
	sc.openLLMSpan = ""
	pending = make([]string, 0, len(sc.openTools))
	for _, spanID := range sc.openTools {
		pending = append(pending, spanID)
	}
	sc.openTools = make(map[string]string)
	rootID = sc.rootID
	sc.mu.Unlock()
	return
}

// OpenToolSpan records a tool call span id.
func (sc *SpanContext) OpenToolSpan(toolCallID, spanID string) {
	sc.mu.Lock()
	sc.openTools[toolCallID] = spanID
	sc.mu.Unlock()
}

// TakeToolSpan removes and returns the span id for a tool call.
func (sc *SpanContext) TakeToolSpan(toolCallID string) (string, bool) {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	spanID, ok := sc.openTools[toolCallID]
	if ok {
		delete(sc.openTools, toolCallID)
	}
	return spanID, ok
}

// HasToolSpan checks if a tool call has an open span.
func (sc *SpanContext) HasToolSpan(toolCallID string) bool {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	_, ok := sc.openTools[toolCallID]
	return ok
}

// SetOpenLLMSpan sets the current open LLM span id.
func (sc *SpanContext) SetOpenLLMSpan(id string) {
	sc.mu.Lock()
	sc.openLLMSpan = id
	sc.mu.Unlock()
}

// MergeLLMSpanTokens updates prompt/completion tokens on the open LLM span.
// Returns true if merged into existing span, false if no open span found.
func (sc *SpanContext) MergeLLMSpanTokens(promptTok, completionTok int) bool {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	if sc.openLLMSpan != "" {
		for i := range sc.spans {
			if sc.spans[i]["id"] == sc.openLLMSpan {
				sc.spans[i]["prompt_tokens"] = promptTok
				sc.spans[i]["completion_tokens"] = completionTok
				return true
			}
		}
	}
	return false
}

// RootID returns the root span id.
func (sc *SpanContext) RootID() string {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	return sc.rootID
}

// Spans returns a copy of the spans slice.
func (sc *SpanContext) Spans() []map[string]any {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	cp := make([]map[string]any, len(sc.spans))
	copy(cp, sc.spans)
	return cp
}

// IterateSpans calls fn for each span while holding the lock.
// The fn callback may mutate the span map directly (it is the original, not a copy).
func (sc *SpanContext) IterateSpans(fn func(i int, span map[string]any)) {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	for i := range sc.spans {
		fn(i, sc.spans[i])
	}
}
