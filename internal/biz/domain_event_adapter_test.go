package biz_test

import (
	"encoding/json"
	"testing"
	"time"

	"aranea-agents/internal/biz"
)

func TestMetaString(t *testing.T) {
	cases := []struct {
		name string
		meta map[string]any
		key  string
		want string
	}{
		{"missing key", map[string]any{"a": "1"}, "b", ""},
		{"nil meta", nil, "a", ""},
		{"string value", map[string]any{"k": "  hello  "}, "k", "hello"},
		{"json number", map[string]any{"k": json.Number("42")}, "k", "42"},
		{"int value", map[string]any{"k": 42}, "k", "42"},
		{"nil value", map[string]any{"k": nil}, "k", ""},
		{"bool value", map[string]any{"k": true}, "k", "true"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := biz.MetaString(tc.meta, tc.key)
			if got != tc.want {
				t.Fatalf("MetaString(%v, %q) = %q, want %q", tc.meta, tc.key, got, tc.want)
			}
		})
	}
}

func TestMetaBool(t *testing.T) {
	cases := []struct {
		name string
		meta map[string]any
		key  string
		want bool
	}{
		{"missing key", map[string]any{"a": "1"}, "b", false},
		{"nil meta", nil, "a", false},
		{"true bool", map[string]any{"k": true}, "k", true},
		{"false bool", map[string]any{"k": false}, "k", false},
		{"string true", map[string]any{"k": "true"}, "k", true},
		{"string True", map[string]any{"k": "True"}, "k", true},
		{"string 1", map[string]any{"k": "1"}, "k", true},
		{"string false", map[string]any{"k": "false"}, "k", false},
		{"string 0", map[string]any{"k": "0"}, "k", false},
		{"nil value", map[string]any{"k": nil}, "k", false},
		{"int value", map[string]any{"k": 1}, "k", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := biz.MetaBool(tc.meta, tc.key)
			if got != tc.want {
				t.Fatalf("MetaBool(%v, %q) = %v, want %v", tc.meta, tc.key, got, tc.want)
			}
		})
	}
}

func TestMetaInt(t *testing.T) {
	cases := []struct {
		name string
		meta map[string]any
		key  string
		want int
	}{
		{"missing key", map[string]any{"a": 1}, "b", 0},
		{"nil meta", nil, "a", 0},
		{"int value", map[string]any{"k": 42}, "k", 42},
		{"int64 value", map[string]any{"k": int64(42)}, "k", 42},
		{"float64 value", map[string]any{"k": float64(42)}, "k", 42},
		{"json number", map[string]any{"k": json.Number("42")}, "k", 42},
		{"string value", map[string]any{"k": "42"}, "k", 42},
		{"invalid string", map[string]any{"k": "abc"}, "k", 0},
		{"nil value", map[string]any{"k": nil}, "k", 0},
		{"zero int", map[string]any{"k": 0}, "k", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := biz.MetaInt(tc.meta, tc.key)
			if got != tc.want {
				t.Fatalf("MetaInt(%v, %q) = %d, want %d", tc.meta, tc.key, got, tc.want)
			}
		})
	}
}

func TestCopyContentToActivity(t *testing.T) {
	src := &biz.DomainContent{Text: "hello", Reasoning: "thinking", IsPartial: true}
	act := &biz.Activity{}
	meta := map[string]any{}
	biz.CopyContentToActivity(src, act, meta)
	if act.Content != "hello" {
		t.Fatalf("Content = %q, want hello", act.Content)
	}
	if act.Reasoning != "thinking" {
		t.Fatalf("Reasoning = %q, want thinking", act.Reasoning)
	}
	if !biz.MetaBool(meta, "has_content") {
		t.Fatalf("has_content should be true")
	}
	if !biz.MetaBool(meta, "is_partial") {
		t.Fatalf("is_partial should be true")
	}
}

func TestCopyContentFromActivity(t *testing.T) {
	t.Run("present", func(t *testing.T) {
		act := biz.Activity{Content: "world", Reasoning: "thought"}
		meta := map[string]any{"has_content": true, "is_partial": false}
		dc := biz.CopyContentFromActivity(act, meta)
		if dc == nil {
			t.Fatalf("expected non-nil DomainContent")
		}
		if dc.Text != "world" {
			t.Fatalf("Text = %q, want world", dc.Text)
		}
		if dc.Reasoning != "thought" {
			t.Fatalf("Reasoning = %q, want thought", dc.Reasoning)
		}
		if dc.IsPartial {
			t.Fatalf("IsPartial should be false")
		}
	})
	t.Run("absent", func(t *testing.T) {
		act := biz.Activity{Content: "x"}
		meta := map[string]any{}
		dc := biz.CopyContentFromActivity(act, meta)
		if dc != nil {
			t.Fatalf("expected nil for absent content")
		}
	})
}

func TestCopyStateDeltaToActivity(t *testing.T) {
	src := &biz.DomainStateDelta{Operation: "set", Path: "a.b", ValueJSON: `{"x":1}`}
	meta := map[string]any{}
	biz.CopyStateDeltaToActivity(src, meta)
	if !biz.MetaBool(meta, "has_state_delta") {
		t.Fatalf("has_state_delta should be true")
	}
	if biz.MetaString(meta, "state_delta_op") != "set" {
		t.Fatalf("op mismatch")
	}
	if biz.MetaString(meta, "state_delta_path") != "a.b" {
		t.Fatalf("path mismatch")
	}
	if biz.MetaString(meta, "state_delta_value_json") != `{"x":1}` {
		t.Fatalf("value mismatch")
	}
}

func TestCopyStateDeltaFromActivity(t *testing.T) {
	t.Run("present", func(t *testing.T) {
		meta := map[string]any{
			"has_state_delta":        true,
			"state_delta_op":         "delete",
			"state_delta_path":       "c.d",
			"state_delta_value_json": "null",
		}
		ds := biz.CopyStateDeltaFromActivity(meta)
		if ds == nil {
			t.Fatalf("expected non-nil DomainStateDelta")
		}
		if ds.Operation != "delete" || ds.Path != "c.d" || ds.ValueJSON != "null" {
			t.Fatalf("unexpected: %+v", ds)
		}
	})
	t.Run("absent", func(t *testing.T) {
		ds := biz.CopyStateDeltaFromActivity(map[string]any{})
		if ds != nil {
			t.Fatalf("expected nil")
		}
	})
}

func TestCopyErrorToActivity(t *testing.T) {
	t.Run("with type", func(t *testing.T) {
		src := &biz.DomainError{Type: "runtime", Message: "boom"}
		act := &biz.Activity{}
		meta := map[string]any{}
		biz.CopyErrorToActivity(src, act, meta)
		if act.ToolErrorCode != "runtime" {
			t.Fatalf("ToolErrorCode = %q, want runtime", act.ToolErrorCode)
		}
		if !biz.MetaBool(meta, "has_error") {
			t.Fatalf("has_error should be true")
		}
		if biz.MetaString(meta, "error_type") != "runtime" {
			t.Fatalf("error_type mismatch")
		}
		if biz.MetaString(meta, "error_message") != "boom" {
			t.Fatalf("error_message mismatch")
		}
	})
	t.Run("empty type", func(t *testing.T) {
		src := &biz.DomainError{Type: "", Message: "oops"}
		act := &biz.Activity{}
		meta := map[string]any{}
		biz.CopyErrorToActivity(src, act, meta)
		if act.ToolErrorCode != "" {
			t.Fatalf("ToolErrorCode should be empty for empty type")
		}
		if !biz.MetaBool(meta, "has_error") {
			t.Fatalf("has_error should still be true")
		}
	})
}

func TestCopyErrorFromActivity(t *testing.T) {
	t.Run("present", func(t *testing.T) {
		act := biz.Activity{}
		meta := map[string]any{
			"has_error":     true,
			"error_type":    "timeout",
			"error_message": "timed out",
		}
		de := biz.CopyErrorFromActivity(act, meta)
		if de == nil {
			t.Fatalf("expected non-nil DomainError")
		}
		if de.Type != "timeout" || de.Message != "timed out" {
			t.Fatalf("unexpected: %+v", de)
		}
	})
	t.Run("absent", func(t *testing.T) {
		de := biz.CopyErrorFromActivity(biz.Activity{}, map[string]any{})
		if de != nil {
			t.Fatalf("expected nil")
		}
	})
}

func TestCopyUsageToActivity(t *testing.T) {
	src := &biz.DomainUsage{PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150}
	act := &biz.Activity{}
	meta := map[string]any{}
	biz.CopyUsageToActivity(src, act, meta)
	if act.PromptTokens != 100 {
		t.Fatalf("PromptTokens = %d, want 100", act.PromptTokens)
	}
	if act.CompletionTokens != 50 {
		t.Fatalf("CompletionTokens = %d, want 50", act.CompletionTokens)
	}
	if !biz.MetaBool(meta, "has_usage") {
		t.Fatalf("has_usage should be true")
	}
	if biz.MetaInt(meta, "usage_total_tokens") != 150 {
		t.Fatalf("total_tokens mismatch")
	}
}

func TestCopyUsageFromActivity(t *testing.T) {
	t.Run("present", func(t *testing.T) {
		act := biz.Activity{PromptTokens: 200, CompletionTokens: 80}
		meta := map[string]any{"has_usage": true, "usage_total_tokens": 280}
		du := biz.CopyUsageFromActivity(act, meta)
		if du == nil {
			t.Fatalf("expected non-nil DomainUsage")
		}
		if du.PromptTokens != 200 || du.CompletionTokens != 80 || du.TotalTokens != 280 {
			t.Fatalf("unexpected: %+v", du)
		}
	})
	t.Run("absent", func(t *testing.T) {
		du := biz.CopyUsageFromActivity(biz.Activity{}, map[string]any{})
		if du != nil {
			t.Fatalf("expected nil")
		}
	})
}

func TestCopyToolCallToActivity(t *testing.T) {
	src := &biz.DomainToolCall{
		ID:            "tc-1",
		Name:          "search",
		ArgumentsJSON: `{"q":"test"}`,
		ResultJSON:    `{"found":true}`,
		Status:        "completed",
		DurationMS:    150,
	}
	act := &biz.Activity{}
	meta := map[string]any{}
	biz.CopyToolCallToActivity(src, act, meta)
	if act.ToolCallID != "tc-1" {
		t.Fatalf("ToolCallID = %q, want tc-1", act.ToolCallID)
	}
	if act.ToolName != "search" {
		t.Fatalf("ToolName = %q, want search", act.ToolName)
	}
	if act.ToolArguments != `{"q":"test"}` {
		t.Fatalf("ToolArguments mismatch")
	}
	if act.ToolResult != `{"found":true}` {
		t.Fatalf("ToolResult mismatch")
	}
	if act.ToolDurationMs != 150 {
		t.Fatalf("ToolDurationMs = %d, want 150", act.ToolDurationMs)
	}
	if !biz.MetaBool(meta, "has_tool_call") {
		t.Fatalf("has_tool_call should be true")
	}
	if biz.MetaString(meta, "tool_call_status") != "completed" {
		t.Fatalf("tool_call_status mismatch")
	}
}

func TestCopyToolCallFromActivity(t *testing.T) {
	t.Run("present", func(t *testing.T) {
		act := biz.Activity{
			ToolCallID:     "tc-2",
			ToolName:       "execute",
			ToolArguments:  `{"cmd":"ls"}`,
			ToolResult:     `{"exit":0}`,
			ToolDurationMs: 50,
		}
		meta := map[string]any{"has_tool_call": true, "tool_call_status": "running"}
		dt := biz.CopyToolCallFromActivity(act, meta)
		if dt == nil {
			t.Fatalf("expected non-nil DomainToolCall")
		}
		if dt.ID != "tc-2" || dt.Name != "execute" || dt.Status != "running" || dt.DurationMS != 50 {
			t.Fatalf("unexpected: %+v", dt)
		}
	})
	t.Run("absent", func(t *testing.T) {
		dt := biz.CopyToolCallFromActivity(biz.Activity{}, map[string]any{})
		if dt != nil {
			t.Fatalf("expected nil")
		}
	})
}

func TestCopyGraphNodeToActivity(t *testing.T) {
	src := &biz.DomainGraphNode{NodeID: "node-1", Error: "fail"}
	meta := map[string]any{}
	biz.CopyGraphNodeToActivity(src, meta)
	if !biz.MetaBool(meta, "has_graph_node") {
		t.Fatalf("has_graph_node should be true")
	}
	if biz.MetaString(meta, "graph_node_id") != "node-1" {
		t.Fatalf("graph_node_id mismatch")
	}
	if biz.MetaString(meta, "graph_node_error") != "fail" {
		t.Fatalf("graph_node_error mismatch")
	}
}

func TestCopyGraphNodeFromActivity(t *testing.T) {
	t.Run("present", func(t *testing.T) {
		meta := map[string]any{
			"has_graph_node":   true,
			"graph_node_id":    "node-2",
			"graph_node_error": "crash",
		}
		gn := biz.CopyGraphNodeFromActivity(meta)
		if gn == nil {
			t.Fatalf("expected non-nil DomainGraphNode")
		}
		if gn.NodeID != "node-2" || gn.Error != "crash" {
			t.Fatalf("unexpected: %+v", gn)
		}
	})
	t.Run("absent", func(t *testing.T) {
		gn := biz.CopyGraphNodeFromActivity(map[string]any{})
		if gn != nil {
			t.Fatalf("expected nil")
		}
	})
}

func TestDomainEventKindStatus(t *testing.T) {
	cases := []struct {
		eventType  biz.DomainEventType
		hasError   bool
		wantKind   biz.ActivityKind
		wantStatus biz.ActivityStatus
	}{
		{biz.DomainEventTextDelta, false, biz.ActivityKindReply, biz.ActivityStatusCompleted},
		{biz.DomainEventToolCall, false, biz.ActivityKindAction, biz.ActivityStatusRunning},
		{biz.DomainEventToolCall, true, biz.ActivityKindAction, biz.ActivityStatusFailed},
		{biz.DomainEventToolResult, false, biz.ActivityKindAction, biz.ActivityStatusCompleted},
		{biz.DomainEventToolResult, true, biz.ActivityKindAction, biz.ActivityStatusFailed},
		{biz.DomainEventRunnerCompletion, false, biz.ActivityKindTask, biz.ActivityStatusCompleted},
		{biz.DomainEventRunnerCompletion, true, biz.ActivityKindTask, biz.ActivityStatusFailed},
		{biz.DomainEventError, false, biz.ActivityKindNotice, biz.ActivityStatusFailed},
		{biz.DomainEventGraphNodeStart, false, biz.ActivityKindGraphStage, biz.ActivityStatusRunning},
		{biz.DomainEventGraphNodeEnd, false, biz.ActivityKindGraphStage, biz.ActivityStatusCompleted},
		{biz.DomainEventGraphNodeError, false, biz.ActivityKindGraphStage, biz.ActivityStatusFailed},
		{biz.DomainEventGraphInterrupt, false, biz.ActivityKindGraphStage, biz.ActivityStatusInterrupted},
		{biz.DomainEventStateDelta, false, biz.ActivityKindNotice, biz.ActivityStatusCompleted},
	}
	for _, tc := range cases {
		t.Run(string(tc.eventType), func(t *testing.T) {
			kind, status := biz.DomainEventKindStatus(tc.eventType, tc.hasError)
			if kind != tc.wantKind {
				t.Fatalf("kind = %q, want %q", kind, tc.wantKind)
			}
			if status != tc.wantStatus {
				t.Fatalf("status = %q, want %q", status, tc.wantStatus)
			}
		})
	}
}

func TestDomainEventToActivityEvent(t *testing.T) {
	t.Run("minimal event", func(t *testing.T) {
		de := biz.DomainEvent{
			Type:      biz.DomainEventTextDelta,
			Author:    "agent-1",
			SessionID: "sess-1",
		}
		ev := biz.DomainEventToActivityEvent(de)
		if ev.Domain != biz.ActivityDomainSystem {
			t.Fatalf("Domain = %q, want system", ev.Domain)
		}
		if ev.Event != biz.ActivityEventCreated {
			t.Fatalf("Event = %q, want created", ev.Event)
		}
		if ev.Activity.Kind != biz.ActivityKindReply {
			t.Fatalf("Kind = %q, want reply", ev.Activity.Kind)
		}
		if ev.Activity.SessionID != "sess-1" {
			t.Fatalf("SessionID = %q, want sess-1", ev.Activity.SessionID)
		}
		if ev.Activity.ID == "" {
			t.Fatalf("ID should not be empty")
		}
		if biz.MetaString(ev.Activity.Meta, "domain_event_type") != "text_delta" {
			t.Fatalf("domain_event_type meta mismatch")
		}
		if biz.MetaString(ev.Activity.Meta, "author") != "agent-1" {
			t.Fatalf("author meta mismatch")
		}
	})

	t.Run("full event with all optional fields", func(t *testing.T) {
		de := biz.DomainEvent{
			Type:       biz.DomainEventRunnerCompletion,
			Author:     "agent-2",
			SessionID:  "sess-2",
			TeamID:     "team-1",
			Content:    &biz.DomainContent{Text: "done", Reasoning: "thought", IsPartial: false},
			Error:      &biz.DomainError{Type: "runtime", Message: "oops"},
			Usage:      &biz.DomainUsage{PromptTokens: 10, CompletionTokens: 20, TotalTokens: 30},
			ToolCall:   &biz.DomainToolCall{ID: "tc-1", Name: "search", Status: "completed", DurationMS: 100},
			StateDelta: &biz.DomainStateDelta{Operation: "set", Path: "x.y", ValueJSON: "1"},
		}
		ev := biz.DomainEventToActivityEvent(de)
		if ev.Activity.TeamID != "team-1" {
			t.Fatalf("TeamID = %q, want team-1", ev.Activity.TeamID)
		}
		if ev.Activity.Kind != biz.ActivityKindTask {
			t.Fatalf("Kind = %q, want task", ev.Activity.Kind)
		}
		if ev.Activity.Status != biz.ActivityStatusFailed {
			t.Fatalf("Status = %q, want failed (has error)", ev.Activity.Status)
		}
		if ev.Activity.Content != "done" {
			t.Fatalf("Content = %q, want done", ev.Activity.Content)
		}
		if ev.Activity.Reasoning != "thought" {
			t.Fatalf("Reasoning = %q, want thought", ev.Activity.Reasoning)
		}
		if ev.Activity.ToolErrorCode != "runtime" {
			t.Fatalf("ToolErrorCode = %q, want runtime", ev.Activity.ToolErrorCode)
		}
		if ev.Activity.PromptTokens != 10 || ev.Activity.CompletionTokens != 20 {
			t.Fatalf("Usage tokens mismatch")
		}
		if biz.MetaInt(ev.Activity.Meta, "usage_total_tokens") != 30 {
			t.Fatalf("total_tokens mismatch")
		}
		if ev.Activity.ToolCallID != "tc-1" || ev.Activity.ToolName != "search" {
			t.Fatalf("ToolCall fields mismatch")
		}
	})
}

func TestActivityEventToDomainEvent(t *testing.T) {
	t.Run("chat domain returns nil", func(t *testing.T) {
		ev := biz.ActivityEvent{
			Domain: biz.ActivityDomainChat,
			Activity: biz.Activity{
				ID:        "a-1",
				SessionID: "s-1",
			},
		}
		de := biz.ActivityEventToDomainEvent(ev)
		if de != nil {
			t.Fatalf("expected nil for chat-domain event")
		}
	})

	t.Run("system domain converts", func(t *testing.T) {
		ev := biz.ActivityEvent{
			Event:  biz.ActivityEventCreated,
			Domain: biz.ActivityDomainSystem,
			Activity: biz.Activity{
				ID:        "act-1",
				SessionID: "sess-1",
				Timestamp: time.Date(2024, 6, 15, 10, 30, 0, 0, time.UTC),
				Meta: map[string]any{
					"domain_event_type": "text_delta",
					"author":            "agent-1",
				},
			},
		}
		de := biz.ActivityEventToDomainEvent(ev)
		if de == nil {
			t.Fatalf("expected non-nil DomainEvent")
		}
		if de.Type != biz.DomainEventTextDelta {
			t.Fatalf("Type = %q, want text_delta", de.Type)
		}
		if de.Author != "agent-1" {
			t.Fatalf("Author = %q, want agent-1", de.Author)
		}
		if de.SessionID != "sess-1" {
			t.Fatalf("SessionID = %q, want sess-1", de.SessionID)
		}
		if de.ID != "act-1" {
			t.Fatalf("ID = %q, want act-1", de.ID)
		}
		if de.Timestamp.Year() != 2024 {
			t.Fatalf("Timestamp year = %d, want 2024", de.Timestamp.Year())
		}
	})

	t.Run("with content via meta marker", func(t *testing.T) {
		ev := biz.ActivityEvent{
			Domain: biz.ActivityDomainSystem,
			Activity: biz.Activity{
				Content:   "hello",
				Reasoning: "think",
				Meta: map[string]any{
					"has_content": true,
					"is_partial":  true,
				},
			},
		}
		de := biz.ActivityEventToDomainEvent(ev)
		if de.Content == nil {
			t.Fatalf("Content should not be nil")
		}
		if de.Content.Text != "hello" {
			t.Fatalf("Content.Text = %q, want hello", de.Content.Text)
		}
		if !de.Content.IsPartial {
			t.Fatalf("IsPartial should be true")
		}
	})

	t.Run("nil meta handled", func(t *testing.T) {
		ev := biz.ActivityEvent{
			Domain: biz.ActivityDomainSystem,
			Activity: biz.Activity{
				ID:   "a-nil",
				Meta: nil,
			},
		}
		de := biz.ActivityEventToDomainEvent(ev)
		if de == nil {
			t.Fatalf("expected non-nil even with nil meta")
		}
		if de.ID != "a-nil" {
			t.Fatalf("ID = %q, want a-nil", de.ID)
		}
	})
}

func TestStoreActivityCorrelation(t *testing.T) {
	de := &biz.DomainEvent{
		Type:          biz.DomainEventToolCall,
		Author:        "bot",
		RequestID:     "req-1",
		InvocationID:  "inv-1",
		RunID:         "run-1",
		TraceID:       "trace-1",
		RunKind:       "chat",
		UsageEventID:  "ue-1",
		TurnStartedAt: time.Date(2024, 6, 15, 10, 0, 0, 0, time.UTC),
	}
	meta := map[string]any{}
	biz.StoreActivityCorrelation(de, meta)
	if biz.MetaString(meta, "domain_event_type") != "tool_call" {
		t.Fatalf("domain_event_type mismatch")
	}
	if biz.MetaString(meta, "author") != "bot" {
		t.Fatalf("author mismatch")
	}
	if biz.MetaString(meta, "request_id") != "req-1" {
		t.Fatalf("request_id mismatch")
	}
	if biz.MetaString(meta, "invocation_id") != "inv-1" {
		t.Fatalf("invocation_id mismatch")
	}
	if biz.MetaString(meta, "run_id") != "run-1" {
		t.Fatalf("run_id mismatch")
	}
	if biz.MetaString(meta, "trace_id") != "trace-1" {
		t.Fatalf("trace_id mismatch")
	}
	if biz.MetaString(meta, "run_kind") != "chat" {
		t.Fatalf("run_kind mismatch")
	}
	if biz.MetaString(meta, "usage_event_id") != "ue-1" {
		t.Fatalf("usage_event_id mismatch")
	}
	if ts := biz.MetaString(meta, "turn_started_at"); ts == "" {
		t.Fatalf("turn_started_at should be set")
	}
}

func TestApplyActivityCorrelation(t *testing.T) {
	t.Run("full correlation extraction", func(t *testing.T) {
		de := &biz.DomainEvent{}
		meta := map[string]any{
			"author":          "bot",
			"request_id":      "req-1",
			"invocation_id":   "inv-1",
			"run_id":          "run-1",
			"trace_id":        "trace-1",
			"run_kind":        "chat",
			"usage_event_id":  "ue-1",
			"turn_started_at": time.Date(2024, 6, 15, 10, 0, 0, 0, time.UTC).Format(time.RFC3339Nano),
		}
		biz.ApplyActivityCorrelation(de, meta)
		if de.Author != "bot" {
			t.Fatalf("Author = %q, want bot", de.Author)
		}
		if de.RequestID != "req-1" {
			t.Fatalf("RequestID = %q, want req-1", de.RequestID)
		}
		if de.InvocationID != "inv-1" {
			t.Fatalf("InvocationID = %q, want inv-1", de.InvocationID)
		}
		if de.RunID != "run-1" {
			t.Fatalf("RunID = %q, want run-1", de.RunID)
		}
		if de.TraceID != "trace-1" {
			t.Fatalf("TraceID = %q, want trace-1", de.TraceID)
		}
		if de.RunKind != "chat" {
			t.Fatalf("RunKind = %q, want chat", de.RunKind)
		}
		if de.UsageEventID != "ue-1" {
			t.Fatalf("UsageEventID = %q, want ue-1", de.UsageEventID)
		}
		if de.TurnStartedAt.Year() != 2024 {
			t.Fatalf("TurnStartedAt year = %d, want 2024", de.TurnStartedAt.Year())
		}
	})

	t.Run("RunID falls back to InvocationID", func(t *testing.T) {
		de := &biz.DomainEvent{}
		meta := map[string]any{
			"invocation_id": "inv-fallback",
		}
		biz.ApplyActivityCorrelation(de, meta)
		if de.RunID != "inv-fallback" {
			t.Fatalf("RunID = %q, want inv-fallback", de.RunID)
		}
	})

	t.Run("nil meta", func(t *testing.T) {
		de := &biz.DomainEvent{}
		biz.ApplyActivityCorrelation(de, nil)
		// should not panic; RunID stays empty since no InvocationID either
		if de.RunID != "" {
			t.Fatalf("RunID should be empty, got %q", de.RunID)
		}
	})

	t.Run("whitespace RequestID trimmed", func(t *testing.T) {
		de := &biz.DomainEvent{}
		meta := map[string]any{
			"request_id": "  req-1  ",
		}
		biz.ApplyActivityCorrelation(de, meta)
		if de.RequestID != "req-1" {
			t.Fatalf("RequestID = %q, want req-1", de.RequestID)
		}
	})
}

func TestDomainEventActivityRoundTrip(t *testing.T) {
	original := biz.DomainEvent{
		ID:               "de-rt-1",
		Type:             biz.DomainEventToolResult,
		Author:           "agent-r",
		SessionID:        "sess-r",
		TeamID:           "team-r",
		Timestamp:        time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC),
		RequestID:        "req-rt",
		InvocationID:     "inv-rt",
		RunID:            "run-rt",
		TraceID:          "trace-rt",
		AgentID:          "agent-rt",
		AgentDisplayName: "TestAgent",
		RunKind:          "chat",
		UsageEventID:     "ue-rt",
		DurationMS:       99,
		Content:          &biz.DomainContent{Text: "response", Reasoning: "logic", IsPartial: false},
		Error:            &biz.DomainError{Type: "test_err", Message: "test msg"},
		Usage:            &biz.DomainUsage{PromptTokens: 5, CompletionTokens: 10, TotalTokens: 15},
		ToolCall:         &biz.DomainToolCall{ID: "tc-r", Name: "tool-r", ArgumentsJSON: `{}`, ResultJSON: `{}`, Status: "done", DurationMS: 42},
		StateDelta:       &biz.DomainStateDelta{Operation: "push", Path: "stack", ValueJSON: `"val"`},
		GraphNode:        &biz.DomainGraphNode{NodeID: "node-r", Error: "node-err"},
	}

	ev := biz.DomainEventToActivityEvent(original)
	if ev.Domain != biz.ActivityDomainSystem {
		t.Fatalf("Domain = %q, want system", ev.Domain)
	}

	recovered := biz.ActivityEventToDomainEvent(ev)
	if recovered == nil {
		t.Fatalf("expected non-nil recovered DomainEvent")
	}

	// Core fields
	if recovered.ID != original.ID {
		t.Fatalf("ID = %q, want %q", recovered.ID, original.ID)
	}
	if recovered.Type != original.Type {
		t.Fatalf("Type = %q, want %q", recovered.Type, original.Type)
	}
	if recovered.Author != original.Author {
		t.Fatalf("Author = %q, want %q", recovered.Author, original.Author)
	}
	if recovered.SessionID != original.SessionID {
		t.Fatalf("SessionID = %q, want %q", recovered.SessionID, original.SessionID)
	}
	if recovered.TeamID != original.TeamID {
		t.Fatalf("TeamID = %q, want %q", recovered.TeamID, original.TeamID)
	}
	if !recovered.Timestamp.Equal(original.Timestamp) {
		t.Fatalf("Timestamp = %v, want %v", recovered.Timestamp, original.Timestamp)
	}

	// Correlation
	if recovered.RequestID != original.RequestID {
		t.Fatalf("RequestID = %q, want %q", recovered.RequestID, original.RequestID)
	}
	if recovered.InvocationID != original.InvocationID {
		t.Fatalf("InvocationID = %q, want %q", recovered.InvocationID, original.InvocationID)
	}
	if recovered.RunID != original.RunID {
		t.Fatalf("RunID = %q, want %q", recovered.RunID, original.RunID)
	}
	if recovered.TraceID != original.TraceID {
		t.Fatalf("TraceID = %q, want %q", recovered.TraceID, original.TraceID)
	}
	if recovered.AgentID != original.AgentID {
		t.Fatalf("AgentID = %q, want %q", recovered.AgentID, original.AgentID)
	}
	if recovered.AgentDisplayName != original.AgentDisplayName {
		t.Fatalf("AgentDisplayName = %q, want %q", recovered.AgentDisplayName, original.AgentDisplayName)
	}
	if recovered.RunKind != original.RunKind {
		t.Fatalf("RunKind = %q, want %q", recovered.RunKind, original.RunKind)
	}
	if recovered.UsageEventID != original.UsageEventID {
		t.Fatalf("UsageEventID = %q, want %q", recovered.UsageEventID, original.UsageEventID)
	}
	if recovered.DurationMS != original.DurationMS {
		t.Fatalf("DurationMS = %d, want %d", recovered.DurationMS, original.DurationMS)
	}

	// Optional structs
	if recovered.Content == nil || recovered.Content.Text != "response" {
		t.Fatalf("Content round-trip failed: %+v", recovered.Content)
	}
	if recovered.Content.Reasoning != "logic" {
		t.Fatalf("Content.Reasoning round-trip failed")
	}
	if recovered.Content.IsPartial {
		t.Fatalf("Content.IsPartial should be false")
	}
	if recovered.Error == nil || recovered.Error.Type != "test_err" {
		t.Fatalf("Error round-trip failed: %+v", recovered.Error)
	}
	if recovered.Error.Message != "test msg" {
		t.Fatalf("Error.Message round-trip failed")
	}
	if recovered.Usage == nil || recovered.Usage.TotalTokens != 15 {
		t.Fatalf("Usage round-trip failed: %+v", recovered.Usage)
	}
	if recovered.Usage.PromptTokens != 5 || recovered.Usage.CompletionTokens != 10 {
		t.Fatalf("Usage tokens round-trip failed")
	}
	if recovered.ToolCall == nil || recovered.ToolCall.Name != "tool-r" {
		t.Fatalf("ToolCall round-trip failed: %+v", recovered.ToolCall)
	}
	if recovered.ToolCall.Status != "done" {
		t.Fatalf("ToolCall.Status round-trip failed")
	}
	if recovered.StateDelta == nil || recovered.StateDelta.Operation != "push" {
		t.Fatalf("StateDelta round-trip failed: %+v", recovered.StateDelta)
	}
	if recovered.GraphNode == nil || recovered.GraphNode.NodeID != "node-r" {
		t.Fatalf("GraphNode round-trip failed: %+v", recovered.GraphNode)
	}
	if recovered.GraphNode.Error != "node-err" {
		t.Fatalf("GraphNode.Error round-trip failed")
	}
}

func TestDomainEventToActivityEventTimestamp(t *testing.T) {
	t.Run("zero timestamp uses now", func(t *testing.T) {
		de := biz.DomainEvent{
			Type: biz.DomainEventTextDelta,
		}
		ev := biz.DomainEventToActivityEvent(de)
		if ev.Activity.Timestamp.IsZero() {
			t.Fatalf("Timestamp should be set to now for zero input")
		}
	})

	t.Run("explicit timestamp preserved", func(t *testing.T) {
		ts := time.Date(2024, 6, 15, 10, 30, 0, 0, time.UTC)
		de := biz.DomainEvent{
			Type:      biz.DomainEventTextDelta,
			Timestamp: ts,
		}
		ev := biz.DomainEventToActivityEvent(de)
		if !ev.Activity.Timestamp.Equal(ts) {
			t.Fatalf("Timestamp = %v, want %v", ev.Activity.Timestamp, ts)
		}
	})
}
