package tracing

import (
	"sync"
	"testing"
	"time"
)

func TestFlowContext_RecordAndTake(t *testing.T) {
	fc := NewFlowContext()
	fc.RecordStart("step1")
	time.Sleep(10 * time.Millisecond)
	timing := fc.TakeTiming("step1")
	if timing == nil {
		t.Fatal("TakeTiming returned nil")
	}
	if timing.DurationMS < 5 {
		t.Errorf("DurationMS = %d, want >= 5", timing.DurationMS)
	}
	// Second take should return nil (consumed)
	if fc.TakeTiming("step1") != nil {
		t.Error("TakeTiming should return nil after consumption")
	}
}

func TestFlowContext_ConcurrentAccess(t *testing.T) {
	fc := NewFlowContext()
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			stepID := string(rune('a' + id%26))
			fc.RecordStart(stepID)
			fc.TakeTiming(stepID)
		}(i)
	}
	wg.Wait()
}

func TestSpanContext_StartEndSpan(t *testing.T) {
	sc := NewSpanContext()
	turnStart := time.Now()
	id := sc.StartSpan("chat.turn", "", nil, "trace-1", turnStart)
	if id != "chat.turn-1" {
		t.Errorf("StartSpan id = %q, want %q", id, "chat.turn-1")
	}
	if sc.RootID() != id {
		t.Errorf("RootID = %q, want %q", sc.RootID(), id)
	}
	time.Sleep(5 * time.Millisecond)
	sc.EndSpan(id, "completed")
	spans := sc.Spans()
	if len(spans) != 1 {
		t.Fatalf("Spans length = %d, want 1", len(spans))
	}
	if spans[0]["status"] != "completed" {
		t.Errorf("status = %v, want completed", spans[0]["status"])
	}
	if spans[0]["duration_ms"].(int64) < 3 {
		t.Errorf("duration_ms = %v, want >= 3", spans[0]["duration_ms"])
	}
}

func TestSpanContext_ParentChild(t *testing.T) {
	sc := NewSpanContext()
	turnStart := time.Now()
	rootID := sc.StartSpan("chat.turn", "", nil, "trace-1", turnStart)
	childID := sc.StartSpan("llm.call", "", nil, "trace-1", turnStart)
	spans := sc.Spans()
	// Child should inherit root as parent when parentID is empty
	if spans[1]["parent_id"] != rootID {
		t.Errorf("child parent_id = %v, want %q", spans[1]["parent_id"], rootID)
	}
	_ = childID
}

func TestSpanContext_EndSpanWithDuration(t *testing.T) {
	sc := NewSpanContext()
	turnStart := time.Now()
	id := sc.StartSpan("tool.call", "", nil, "trace-1", turnStart)
	sc.EndSpanWithDuration(id, "completed", 150)
	spans := sc.Spans()
	if spans[0]["duration_ms"].(int64) != 150 {
		t.Errorf("duration_ms = %v, want 150", spans[0]["duration_ms"])
	}
}

func TestSpanContext_ToolSpan(t *testing.T) {
	sc := NewSpanContext()
	turnStart := time.Now()
	spanID := sc.StartSpan("tool.call", "", map[string]any{"tool_call_id": "tc-1"}, "trace-1", turnStart)
	sc.OpenToolSpan("tc-1", spanID)
	if !sc.HasToolSpan("tc-1") {
		t.Error("HasToolSpan should return true")
	}
	got, ok := sc.TakeToolSpan("tc-1")
	if !ok {
		t.Error("TakeToolSpan should return true")
	}
	if got != spanID {
		t.Errorf("TakeToolSpan = %q, want %q", got, spanID)
	}
	if sc.HasToolSpan("tc-1") {
		t.Error("HasToolSpan should return false after TakeToolSpan")
	}
}

func TestSpanContext_LLMSpan(t *testing.T) {
	sc := NewSpanContext()
	turnStart := time.Now()
	id := sc.StartSpan("llm.call", "", nil, "trace-1", turnStart)
	sc.SetOpenLLMSpan(id)
	if !sc.MergeLLMSpanTokens(100, 50) {
		t.Error("MergeLLMSpanTokens should return true")
	}
	spans := sc.Spans()
	if spans[0]["prompt_tokens"] != 100 {
		t.Errorf("prompt_tokens = %v, want 100", spans[0]["prompt_tokens"])
	}
	if spans[0]["completion_tokens"] != 50 {
		t.Errorf("completion_tokens = %v, want 50", spans[0]["completion_tokens"])
	}
}

func TestSpanContext_FinishRoot(t *testing.T) {
	sc := NewSpanContext()
	turnStart := time.Now()
	rootID := sc.StartSpan("chat.turn", "", nil, "trace-1", turnStart)
	llmID := sc.StartSpan("llm.call", "", nil, "trace-1", turnStart)
	sc.SetOpenLLMSpan(llmID)
	toolID := sc.StartSpan("tool.call", "", map[string]any{"tool_call_id": "tc-1"}, "trace-1", turnStart)
	sc.OpenToolSpan("tc-1", toolID)

	gotLLMID, pending, gotRootID := sc.FinishRoot("completed")
	if gotLLMID != llmID {
		t.Errorf("llmID = %q, want %q", gotLLMID, llmID)
	}
	if len(pending) != 1 || pending[0] != toolID {
		t.Errorf("pending = %v, want [%q]", pending, toolID)
	}
	if gotRootID != rootID {
		t.Errorf("rootID = %q, want %q", gotRootID, rootID)
	}
}

func TestSpanContext_IterateSpans(t *testing.T) {
	sc := NewSpanContext()
	turnStart := time.Now()
	sc.StartSpan("chat.turn", "", nil, "trace-1", turnStart)
	sc.StartSpan("llm.call", "", nil, "trace-1", turnStart)
	count := 0
	sc.IterateSpans(func(i int, span map[string]any) {
		count++
		span["custom"] = true
	})
	if count != 2 {
		t.Errorf("IterateSpans count = %d, want 2", count)
	}
	spans := sc.Spans()
	if spans[0]["custom"] != true {
		t.Error("IterateSpans should allow mutation")
	}
}

func TestUsageContext_OtelRefs(t *testing.T) {
	uc := NewUsageContext()
	uc.SetOtelRefs("trace-abc", "span-123")
	if uc.OtelTraceID() != "trace-abc" {
		t.Errorf("OtelTraceID = %q, want %q", uc.OtelTraceID(), "trace-abc")
	}
	if uc.OtelRootID() != "span-123" {
		t.Errorf("OtelRootID = %q, want %q", uc.OtelRootID(), "span-123")
	}
}

func TestUsageContext_TurnStart(t *testing.T) {
	uc := NewUsageContext()
	before := time.Now()
	ts := uc.TurnStart()
	after := time.Now()
	if ts.Before(before) || ts.After(after) {
		t.Errorf("TurnStart = %v, want between %v and %v", ts, before, after)
	}
}

func TestUsageContext_ConcurrentAccess(t *testing.T) {
	uc := NewUsageContext()
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			uc.SetOtelRefs("trace", "span")
			_ = uc.OtelTraceID()
			_ = uc.OtelRootID()
			_ = uc.TurnStart()
		}(i)
	}
	wg.Wait()
}
