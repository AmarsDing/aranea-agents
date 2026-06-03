package event

import (
	"encoding/json"
	"strings"
)

// SpanCollector manages the span tree using a SpanContext.
type SpanCollector struct {
	sc *SpanContext
	uc *UsageContext
	tc TraceContext
}

// NewSpanCollector creates a SpanCollector and opens the root chat.turn span.
func NewSpanCollector(sc *SpanContext, uc *UsageContext, tc TraceContext) *SpanCollector {
	col := &SpanCollector{sc: sc, uc: uc, tc: tc}
	col.startSpan("chat.turn", "", map[string]any{
		"trace_id":  tc.TraceID,
		"run_id":    tc.RunID,
		"agent_key": tc.AgentKey,
	})
	return col
}

func (c *SpanCollector) startSpan(name, parentID string, attrs map[string]any) string {
	if c == nil {
		return ""
	}
	return c.sc.StartSpan(name, parentID, attrs, c.tc.TraceID, c.uc.TurnStart())
}

func (c *SpanCollector) endSpan(id, status string) {
	if c == nil || id == "" {
		return
	}
	c.sc.EndSpan(id, status)
}

func (c *SpanCollector) endSpanWithDuration(id, status string, durationMS int) {
	if c == nil || id == "" {
		return
	}
	c.sc.EndSpanWithDuration(id, status, durationMS)
}

// FinishRoot closes all open spans and the root span.
func (c *SpanCollector) FinishRoot(status string) {
	if c == nil {
		return
	}
	llmID, pending, rootID := c.sc.FinishRoot(status)
	if llmID != "" {
		c.endSpan(llmID, status)
	}
	for _, spanID := range pending {
		c.endSpan(spanID, status)
	}
	c.endSpan(rootID, status)
}

// CompleteToolCall ends a tool.call usage span using hook-measured duration.
func (c *SpanCollector) CompleteToolCall(toolCallID, toolName string, durationMS int, status string) {
	if c == nil || toolCallID == "" {
		return
	}
	spanID, ok := c.sc.TakeToolSpan(toolCallID)
	if !ok {
		spanID = c.startSpan("tool.call", c.sc.RootID(), map[string]any{
			"tool_name":    toolName,
			"tool_call_id": toolCallID,
		})
	}
	c.endSpanWithDuration(spanID, normalizeSpanStatus(status), durationMS)
}

// OpenToolSpan creates a tool.call span and registers it as open.
func (c *SpanCollector) OpenToolSpan(name, toolCallID string) {
	if c == nil {
		return
	}
	spanID := c.startSpan("tool.call", c.sc.RootID(), map[string]any{
		"tool_name":    name,
		"tool_call_id": toolCallID,
	})
	c.sc.OpenToolSpan(toolCallID, spanID)
}

// HasToolSpan checks if a tool call has an open span.
func (c *SpanCollector) HasToolSpan(toolCallID string) bool {
	if c == nil {
		return false
	}
	return c.sc.HasToolSpan(toolCallID)
}

// StartLLMSpan creates or merges an LLM span.
func (c *SpanCollector) StartLLMSpan(promptTok, completionTok int) {
	if c == nil {
		return
	}
	if c.sc.MergeLLMSpanTokens(promptTok, completionTok) {
		return
	}
	id := c.startSpan("llm.call", c.sc.RootID(), map[string]any{
		"prompt_tokens":     promptTok,
		"completion_tokens": completionTok,
	})
	c.sc.SetOpenLLMSpan(id)
}

// SyncOtelSpanIDs copies OTel child span ids onto usage waterfall rows.
func (c *SpanCollector) SyncOtelSpanIDs(src OtelSpanIDSource) {
	if c == nil || src == nil {
		return
	}
	otelRootID := c.uc.OtelRootID()
	c.sc.IterateSpans(func(_ int, span map[string]any) {
		name, _ := span["name"].(string)
		switch name {
		case "llm.call":
			if id := src.LLMSpanOtelID(); id != "" {
				span["otel_id"] = id
			}
		case "tool.call":
			tid, _ := span["tool_call_id"].(string)
			if id := src.ToolSpanOtelID(tid); id != "" {
				span["otel_id"] = id
			}
		case "chat.turn":
			if otelRootID != "" {
				span["otel_id"] = otelRootID
			}
		}
	})
}

// MetadataJSON returns spans + trace_id for usage.metadata_json.
func (c *SpanCollector) MetadataJSON() string {
	if c == nil {
		return "{}"
	}
	spans := c.sc.Spans()
	otelTraceID := c.uc.OtelTraceID()
	otelRootID := c.uc.OtelRootID()
	turnStart := c.uc.TurnStart()
	payload := map[string]any{
		"trace_id":      c.tc.TraceID,
		"spans":         spans,
		"trace_root_ms": turnStart.UnixMilli(),
	}
	if otelTraceID != "" {
		payload["otel_trace_id"] = otelTraceID
	}
	if otelRootID != "" {
		payload["otel_root_span_id"] = otelRootID
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "{}"
	}
	return string(raw)
}

// Spans returns the raw spans slice (for test access).
func (c *SpanCollector) Spans() []map[string]any {
	if c == nil {
		return nil
	}
	return c.sc.Spans()
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
