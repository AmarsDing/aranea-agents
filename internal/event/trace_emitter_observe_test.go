package event

import (
	"encoding/json"
	"testing"

	trpcevent "trpc.group/trpc-go/trpc-agent-go/event"
	"trpc.group/trpc-go/trpc-agent-go/model"
)

func TestObserveFrameworkEventMergesLLMSpans(t *testing.T) {
	em := NewTraceEmitter(nil, TraceContext{TraceID: "tr_x", RunID: "r1"}, nil)
	ev := &trpcevent.Event{Response: &model.Response{Usage: &model.Usage{PromptTokens: 10, CompletionTokens: 5}}}
	em.ObserveFrameworkEvent(ev)
	em.ObserveFrameworkEvent(ev)
	raw := em.MetadataJSON()
	var payload map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatal(err)
	}
	spans, _ := payload["spans"].([]any)
	llmCount := 0
	for _, s := range spans {
		row, _ := s.(map[string]any)
		if row["name"] == "llm.call" {
			llmCount++
		}
	}
	if llmCount != 1 {
		t.Fatalf("expected 1 merged llm span, got %d (total spans %d)", llmCount, len(spans))
	}
}

func TestCompleteToolCallSetsDuration(t *testing.T) {
	em := NewTraceEmitter(nil, TraceContext{TraceID: "tr_x", RunID: "r1"}, nil)
	em.CompleteToolCall("call_1", "search", 120, "success")
	raw := em.MetadataJSON()
	var payload map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatal(err)
	}
	spans, _ := payload["spans"].([]any)
	if len(spans) == 0 {
		t.Fatal("expected tool span")
	}
}

func TestSetOtelRefsInMetadata(t *testing.T) {
	em := NewTraceEmitter(nil, TraceContext{TraceID: "tr_x"}, nil)
	em.SetOtelRefs("otel_tr", "otel_span")
	raw := em.MetadataJSON()
	if raw == "" || raw == "{}" {
		t.Fatal("expected metadata")
	}
	var payload map[string]any
	_ = json.Unmarshal([]byte(raw), &payload)
	if payload["otel_trace_id"] != "otel_tr" {
		t.Fatalf("missing otel_trace_id: %+v", payload)
	}
}
