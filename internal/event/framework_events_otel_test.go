package event

import "testing"

func TestSyncOtelSpanIDsRoot(t *testing.T) {
	em := NewTraceEmitter(TraceContext{TraceID: "t1", SessionID: "s1"}, nil)
	em.SetOtelRefs("trace-1", "root-span-1")
	em.SyncOtelSpanIDs(&otelStub{llm: "llm-span"})
	spans := em.SpanCollector().Spans()
	if len(spans) == 0 {
		t.Fatal("expected spans")
	}
	found := false
	for _, row := range spans {
		if id, _ := row["otel_id"].(string); id == "root-span-1" || id == "llm-span" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected otel_id on span rows: %+v", spans)
	}
}

type otelStub struct {
	llm   string
	tools map[string]string
}

func (o *otelStub) LLMSpanOtelID() string           { return o.llm }
func (o *otelStub) ToolSpanOtelID(id string) string { return o.tools[id] }
