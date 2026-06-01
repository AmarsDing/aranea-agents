package biz_test

import (
	"encoding/json"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event/contract"
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

func TestCopyContentToEnvelope(t *testing.T) {
	src := &biz.DomainContent{Text: "hello", Reasoning: "thinking", IsPartial: true}
	dst := &contract.EnvelopeContent{}
	biz.CopyContentToEnvelope(src, dst)
	if dst.Text != "hello" {
		t.Fatalf("Text = %q, want hello", dst.Text)
	}
	if dst.Reasoning != "thinking" {
		t.Fatalf("Reasoning = %q, want thinking", dst.Reasoning)
	}
	if !dst.IsPartial {
		t.Fatalf("IsPartial should be true")
	}
}

func TestCopyContentFromEnvelope(t *testing.T) {
	src := &contract.EnvelopeContent{Text: "world", Reasoning: "thought", IsPartial: false}
	dst := &biz.DomainContent{}
	biz.CopyContentFromEnvelope(src, dst)
	if dst.Text != "world" {
		t.Fatalf("Text = %q, want world", dst.Text)
	}
	if dst.Reasoning != "thought" {
		t.Fatalf("Reasoning = %q, want thought", dst.Reasoning)
	}
	if dst.IsPartial {
		t.Fatalf("IsPartial should be false")
	}
}

func TestCopyStateDeltaToEnvelope(t *testing.T) {
	src := &biz.DomainStateDelta{Operation: "set", Path: "a.b", ValueJSON: `{"x":1}`}
	dst := &contract.EnvelopeStateDelta{}
	biz.CopyStateDeltaToEnvelope(src, dst)
	if dst.Operation != "set" || dst.Path != "a.b" || dst.ValueJSON != `{"x":1}` {
		t.Fatalf("unexpected dst: %+v", dst)
	}
}

func TestCopyStateDeltaFromEnvelope(t *testing.T) {
	src := &contract.EnvelopeStateDelta{Operation: "delete", Path: "c.d", ValueJSON: "null"}
	dst := &biz.DomainStateDelta{}
	biz.CopyStateDeltaFromEnvelope(src, dst)
	if dst.Operation != "delete" || dst.Path != "c.d" {
		t.Fatalf("unexpected dst: %+v", dst)
	}
}

func TestCopyErrorToEnvelope(t *testing.T) {
	src := &biz.DomainError{Type: "runtime", Message: "boom"}
	dst := &contract.EnvelopeError{}
	biz.CopyErrorToEnvelope(src, dst)
	if dst.Type != "runtime" || dst.Message != "boom" {
		t.Fatalf("unexpected dst: %+v", dst)
	}
}

func TestCopyErrorFromEnvelope(t *testing.T) {
	src := &contract.EnvelopeError{Type: "timeout", Message: "timed out"}
	dst := &biz.DomainError{}
	biz.CopyErrorFromEnvelope(src, dst)
	if dst.Type != "timeout" || dst.Message != "timed out" {
		t.Fatalf("unexpected dst: %+v", dst)
	}
}

func TestCopyUsageToEnvelope(t *testing.T) {
	src := &biz.DomainUsage{PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150}
	dst := &contract.EnvelopeUsage{}
	biz.CopyUsageToEnvelope(src, dst)
	if dst.PromptTokens != 100 || dst.CompletionTokens != 50 || dst.TotalTokens != 150 {
		t.Fatalf("unexpected dst: %+v", dst)
	}
}

func TestCopyUsageFromEnvelope(t *testing.T) {
	src := &contract.EnvelopeUsage{PromptTokens: 200, CompletionTokens: 80, TotalTokens: 280}
	dst := &biz.DomainUsage{}
	biz.CopyUsageFromEnvelope(src, dst)
	if dst.PromptTokens != 200 || dst.CompletionTokens != 80 || dst.TotalTokens != 280 {
		t.Fatalf("unexpected dst: %+v", dst)
	}
}

func TestCopyToolCallToEnvelope(t *testing.T) {
	src := &biz.DomainToolCall{
		ID:            "tc-1",
		Name:          "search",
		ArgumentsJSON: `{"q":"test"}`,
		ResultJSON:    `{"found":true}`,
		Status:        "completed",
		DurationMS:    150,
	}
	dst := &contract.EnvelopeToolCall{}
	biz.CopyToolCallToEnvelope(src, dst)
	if dst.ID != "tc-1" || dst.Name != "search" || dst.Status != "completed" || dst.DurationMS != 150 {
		t.Fatalf("unexpected dst: %+v", dst)
	}
}

func TestCopyToolCallFromEnvelope(t *testing.T) {
	src := &contract.EnvelopeToolCall{
		ID:            "tc-2",
		Name:          "execute",
		ArgumentsJSON: `{"cmd":"ls"}`,
		ResultJSON:    `{"exit":0}`,
		Status:        "running",
		DurationMS:    50,
	}
	dst := &biz.DomainToolCall{}
	biz.CopyToolCallFromEnvelope(src, dst)
	if dst.ID != "tc-2" || dst.Name != "execute" || dst.Status != "running" || dst.DurationMS != 50 {
		t.Fatalf("unexpected dst: %+v", dst)
	}
}

func TestDomainEventToEnvelope(t *testing.T) {
	t.Run("minimal event", func(t *testing.T) {
		de := biz.DomainEvent{
			Type:      biz.DomainEventTextDelta,
			Author:    "agent-1",
			SessionID: "sess-1",
		}
		env := biz.DomainEventToEnvelope(de)
		if env.Type != contract.EnvelopeTypeTextDelta {
			t.Fatalf("Type = %q, want text_delta", env.Type)
		}
		if env.Author != "agent-1" {
			t.Fatalf("Author = %q, want agent-1", env.Author)
		}
		if env.SessionID != "sess-1" {
			t.Fatalf("SessionID = %q, want sess-1", env.SessionID)
		}
		if env.ID == "" {
			t.Fatalf("ID should not be empty")
		}
	})

	t.Run("full event with all optional fields", func(t *testing.T) {
		de := biz.DomainEvent{
			Type:      biz.DomainEventRunnerCompletion,
			Author:    "agent-2",
			SessionID: "sess-2",
			TeamID:    "team-1",
			Content:   &biz.DomainContent{Text: "done", Reasoning: "thought", IsPartial: false},
			Error:     &biz.DomainError{Type: "runtime", Message: "oops"},
			Usage:     &biz.DomainUsage{PromptTokens: 10, CompletionTokens: 20, TotalTokens: 30},
			ToolCall:  &biz.DomainToolCall{ID: "tc-1", Name: "search", Status: "completed", DurationMS: 100},
			StateDelta: &biz.DomainStateDelta{Operation: "set", Path: "x.y", ValueJSON: "1"},
		}
		env := biz.DomainEventToEnvelope(de)
		if env.TeamID != "team-1" {
			t.Fatalf("TeamID = %q, want team-1", env.TeamID)
		}
		if env.Content == nil || env.Content.Text != "done" {
			t.Fatalf("Content not copied correctly")
		}
		if env.Error == nil || env.Error.Message != "oops" {
			t.Fatalf("Error not copied correctly")
		}
		if env.Usage == nil || env.Usage.TotalTokens != 30 {
			t.Fatalf("Usage not copied correctly")
		}
		if env.ToolCall == nil || env.ToolCall.Name != "search" {
			t.Fatalf("ToolCall not copied correctly")
		}
		if env.StateDelta == nil || env.StateDelta.Path != "x.y" {
			t.Fatalf("StateDelta not copied correctly")
		}
	})
}

func TestEnvelopeToDomainEvent(t *testing.T) {
	t.Run("minimal envelope", func(t *testing.T) {
		env := contract.NewEnvelope(contract.EnvelopeTypeTextDelta, "agent-1", "sess-1")
		de := biz.EnvelopeToDomainEvent(env)
		if de == nil {
			t.Fatalf("expected non-nil DomainEvent")
		}
		if de.Author != "agent-1" {
			t.Fatalf("Author = %q, want agent-1", de.Author)
		}
		if de.SessionID != "sess-1" {
			t.Fatalf("SessionID = %q, want sess-1", de.SessionID)
		}
		if de.Timestamp.IsZero() {
			t.Fatalf("Timestamp should not be zero")
		}
	})

	t.Run("envelope with content", func(t *testing.T) {
		env := contract.NewEnvelope(contract.EnvelopeTypeTextDelta, "agent-1", "sess-1")
		env.Content = &contract.EnvelopeContent{Text: "hello", Reasoning: "think", IsPartial: true}
		de := biz.EnvelopeToDomainEvent(env)
		if de.Content == nil || de.Content.Text != "hello" {
			t.Fatalf("Content not copied correctly")
		}
	})

	t.Run("envelope with error", func(t *testing.T) {
		env := contract.NewEnvelope(contract.EnvelopeTypeError, "agent-1", "sess-1")
		env.Error = &contract.EnvelopeError{Type: "timeout", Message: "timed out"}
		de := biz.EnvelopeToDomainEvent(env)
		if de.Error == nil || de.Error.Type != "timeout" {
			t.Fatalf("Error not copied correctly")
		}
	})

	t.Run("envelope with usage", func(t *testing.T) {
		env := contract.NewEnvelope(contract.EnvelopeTypeRunnerCompletion, "agent-1", "sess-1")
		env.Usage = &contract.EnvelopeUsage{PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150}
		de := biz.EnvelopeToDomainEvent(env)
		if de.Usage == nil || de.Usage.TotalTokens != 150 {
			t.Fatalf("Usage not copied correctly")
		}
	})

	t.Run("envelope with tool call", func(t *testing.T) {
		env := contract.NewEnvelope(contract.EnvelopeTypeToolCall, "agent-1", "sess-1")
		env.ToolCall = &contract.EnvelopeToolCall{ID: "tc-1", Name: "search", Status: "completed", DurationMS: 200}
		de := biz.EnvelopeToDomainEvent(env)
		if de.ToolCall == nil || de.ToolCall.Name != "search" {
			t.Fatalf("ToolCall not copied correctly")
		}
	})

	t.Run("envelope with state delta", func(t *testing.T) {
		env := contract.NewEnvelope(contract.EnvelopeTypeStateDelta, "agent-1", "sess-1")
		env.StateDelta = &contract.EnvelopeStateDelta{Operation: "set", Path: "a.b", ValueJSON: `"v"`}
		de := biz.EnvelopeToDomainEvent(env)
		if de.StateDelta == nil || de.StateDelta.Operation != "set" {
			t.Fatalf("StateDelta not copied correctly")
		}
	})
}

func TestApplyEnvelopeCorrelation(t *testing.T) {
	t.Run("metadata extraction", func(t *testing.T) {
		de := &biz.DomainEvent{}
		env := contract.Envelope{
			RequestID:    "req-1",
			InvocationID: "inv-1",
			Metadata: map[string]any{
				"run_id":             "run-1",
				"trace_id":           "trace-1",
				"agent_id":           "agent-1",
				"agent_display_name": "TestAgent",
				"run_kind":           "chat",
				"usage_event_id":     "ue-1",
			},
		}
		biz.ApplyEnvelopeCorrelation(de, env)
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
		if de.AgentID != "agent-1" {
			t.Fatalf("AgentID = %q, want agent-1", de.AgentID)
		}
		if de.AgentDisplayName != "TestAgent" {
			t.Fatalf("AgentDisplayName = %q, want TestAgent", de.AgentDisplayName)
		}
		if de.RunKind != "chat" {
			t.Fatalf("RunKind = %q, want chat", de.RunKind)
		}
		if de.UsageEventID != "ue-1" {
			t.Fatalf("UsageEventID = %q, want ue-1", de.UsageEventID)
		}
	})

	t.Run("RunID falls back to InvocationID", func(t *testing.T) {
		de := &biz.DomainEvent{}
		env := contract.Envelope{
			InvocationID: "inv-fallback",
			Metadata:     map[string]any{},
		}
		biz.ApplyEnvelopeCorrelation(de, env)
		if de.RunID != "inv-fallback" {
			t.Fatalf("RunID = %q, want inv-fallback", de.RunID)
		}
	})

	t.Run("nil metadata", func(t *testing.T) {
		de := &biz.DomainEvent{}
		env := contract.Envelope{
			InvocationID: "inv-1",
		}
		biz.ApplyEnvelopeCorrelation(de, env)
		if de.RunID != "inv-1" {
			t.Fatalf("RunID should fall back to InvocationID, got %q", de.RunID)
		}
	})

	t.Run("whitespace RequestID trimmed", func(t *testing.T) {
		de := &biz.DomainEvent{}
		env := contract.Envelope{
			RequestID: "  req-1  ",
		}
		biz.ApplyEnvelopeCorrelation(de, env)
		if de.RequestID != "req-1" {
			t.Fatalf("RequestID = %q, want req-1", de.RequestID)
		}
	})
}

func TestDomainEventToEnvelopeRoundTrip(t *testing.T) {
	original := biz.DomainEvent{
		Type:      biz.DomainEventToolCall,
		Author:    "agent-r",
		SessionID: "sess-r",
		TeamID:    "team-r",
		Content:   &biz.DomainContent{Text: "response", Reasoning: "logic", IsPartial: false},
		Error:     &biz.DomainError{Type: "test_err", Message: "test msg"},
		Usage:     &biz.DomainUsage{PromptTokens: 5, CompletionTokens: 10, TotalTokens: 15},
		ToolCall:  &biz.DomainToolCall{ID: "tc-r", Name: "tool-r", ArgumentsJSON: `{}`, ResultJSON: `{}`, Status: "done", DurationMS: 42},
		StateDelta: &biz.DomainStateDelta{Operation: "push", Path: "stack", ValueJSON: `"val"`},
	}

	env := biz.DomainEventToEnvelope(original)
	recovered := biz.EnvelopeToDomainEvent(env)

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
	if recovered.Content == nil || recovered.Content.Text != "response" {
		t.Fatalf("Content round-trip failed")
	}
	if recovered.Error == nil || recovered.Error.Type != "test_err" {
		t.Fatalf("Error round-trip failed")
	}
	if recovered.Usage == nil || recovered.Usage.TotalTokens != 15 {
		t.Fatalf("Usage round-trip failed")
	}
	if recovered.ToolCall == nil || recovered.ToolCall.Name != "tool-r" {
		t.Fatalf("ToolCall round-trip failed")
	}
	if recovered.StateDelta == nil || recovered.StateDelta.Operation != "push" {
		t.Fatalf("StateDelta round-trip failed")
	}
}

func TestEnvelopeToDomainEventTimestamp(t *testing.T) {
	t.Run("valid timestamp", func(t *testing.T) {
		ts := time.Date(2024, 6, 15, 10, 30, 0, 0, time.UTC).Format(time.RFC3339Nano)
		env := contract.NewEnvelope(contract.EnvelopeTypeTextDelta, "a", "s")
		env.Timestamp = ts
		de := biz.EnvelopeToDomainEvent(env)
		if de.Timestamp.Year() != 2024 {
			t.Fatalf("Timestamp year = %d, want 2024", de.Timestamp.Year())
		}
	})

	t.Run("invalid timestamp uses now", func(t *testing.T) {
		env := contract.NewEnvelope(contract.EnvelopeTypeTextDelta, "a", "s")
		env.Timestamp = "not-a-timestamp"
		de := biz.EnvelopeToDomainEvent(env)
		if de.Timestamp.IsZero() {
			t.Fatalf("Timestamp should be set to now for invalid input")
		}
	})

	t.Run("empty timestamp uses now", func(t *testing.T) {
		env := contract.NewEnvelope(contract.EnvelopeTypeTextDelta, "a", "s")
		env.Timestamp = ""
		de := biz.EnvelopeToDomainEvent(env)
		if de.Timestamp.IsZero() {
			t.Fatalf("Timestamp should be set to now for empty input")
		}
	})
}
