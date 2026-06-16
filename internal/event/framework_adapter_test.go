package event

import (
	"encoding/json"
	"testing"
	"time"

	"aranea-agents/internal/event/contract"

	trpcevent "trpc.group/trpc-go/trpc-agent-go/event"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

func TestFromFrameworkEvent_BasicFields(t *testing.T) {
	ev := &trpcevent.Event{
		ID:           "evt-123",
		InvocationID: "inv-456",
		Author:       "agent-1",
		Branch:       "main",
		FilterKey:    "session/abc",
		Tag:          "code_execution;transfer",
		Version:      2,
		Timestamp:    time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC),
	}
	ev.RequestID = "req-789"
	ev.ParentInvocationID = "parent-inv"

	meta := FrameworkEventMeta{
		SessionID:          "sess-001",
		RequestID:          "meta-req",
		InvocationID:       "meta-inv",
		ParentInvocationID: "meta-parent",
		TeamID:             "team-1",
		Branch:             "meta-branch",
		FilterKey:          "meta/filter",
		Source:             "test",
	}

	env := FromFrameworkEvent(ev, meta, EnvelopeTypeTextDelta)

	// Framework event fields take precedence
	if env.ID != "evt-123" {
		t.Errorf("ID = %q, want %q", env.ID, "evt-123")
	}
	if env.RequestID != "req-789" {
		t.Errorf("RequestID = %q, want %q", env.RequestID, "req-789")
	}
	if env.InvocationID != "meta-inv" {
		t.Errorf("InvocationID = %q, want %q (meta overrides when framework is empty)", env.InvocationID, "meta-inv")
	}
	if env.ParentInvocationID != "meta-parent" {
		t.Errorf("ParentInvocationID = %q, want %q (meta overrides when framework is empty)", env.ParentInvocationID, "meta-parent")
	}
	if env.Branch != "main" {
		t.Errorf("Branch = %q, want %q (framework takes precedence)", env.Branch, "main")
	}
	if env.FilterKey != "session/abc" {
		t.Errorf("FilterKey = %q, want %q (framework takes precedence)", env.FilterKey, "session/abc")
	}
	if env.Tag != "code_execution;transfer" {
		t.Errorf("Tag = %q, want %q", env.Tag, "code_execution;transfer")
	}
	if env.TeamID != "team-1" {
		t.Errorf("TeamID = %q, want %q", env.TeamID, "team-1")
	}
	if env.Version != 2 {
		t.Errorf("Version = %d, want %d", env.Version, 2)
	}
	if env.Source != "test" {
		t.Errorf("Source = %q, want %q", env.Source, "test")
	}
	if env.Type != EnvelopeTypeTextDelta {
		t.Errorf("Type = %q, want %q", env.Type, EnvelopeTypeTextDelta)
	}
	if env.Author != "agent-1" {
		t.Errorf("Author = %q, want %q", env.Author, "agent-1")
	}
	if env.SessionID != "sess-001" {
		t.Errorf("SessionID = %q, want %q", env.SessionID, "sess-001")
	}
}

func TestFromFrameworkEvent_MetaFallback(t *testing.T) {
	// When framework event has empty Branch/FilterKey, meta values are used.
	ev := &trpcevent.Event{
		Author: "agent",
	}
	meta := FrameworkEventMeta{
		SessionID: "sess-1",
		Branch:    "meta-branch",
		FilterKey: "meta/filter",
	}

	env := FromFrameworkEvent(ev, meta, EnvelopeTypeToolCall)

	if env.Branch != "meta-branch" {
		t.Errorf("Branch = %q, want %q (meta fallback)", env.Branch, "meta-branch")
	}
	if env.FilterKey != "meta/filter" {
		t.Errorf("FilterKey = %q, want %q (meta fallback)", env.FilterKey, "meta/filter")
	}
}

func TestFromFrameworkEvent_Extensions(t *testing.T) {
	ev := &trpcevent.Event{
		Author: "agent",
		Extensions: map[string]json.RawMessage{
			"simple_string": json.RawMessage(`"hello"`),
			"complex_json":  json.RawMessage(`{"key":"value"}`),
			"number":        json.RawMessage(`42`),
		},
	}

	env := FromFrameworkEvent(ev, FrameworkEventMeta{SessionID: "s"}, EnvelopeTypeTextDelta)

	if env.Extensions["simple_string"] != "hello" {
		t.Errorf("simple_string = %q, want %q (quotes stripped)", env.Extensions["simple_string"], "hello")
	}
	if env.Extensions["complex_json"] != `{"key":"value"}` {
		t.Errorf("complex_json = %q, want raw JSON preserved", env.Extensions["complex_json"])
	}
	if env.Extensions["number"] != "42" {
		t.Errorf("number = %q, want %q", env.Extensions["number"], "42")
	}
}

func TestFromFrameworkEvent_Actions(t *testing.T) {
	ev := &trpcevent.Event{
		Author: "agent",
		Actions: &trpcevent.EventActions{
			SkipSummarization: true,
		},
	}

	env := FromFrameworkEvent(ev, FrameworkEventMeta{SessionID: "s"}, EnvelopeTypeStateDelta)

	if env.Actions == nil {
		t.Fatal("Actions is nil, want non-nil")
	}
	if !env.Actions.SkipSummarization {
		t.Error("Actions.SkipSummarization = false, want true")
	}
}

func TestFromFrameworkEvent_NilActions(t *testing.T) {
	ev := &trpcevent.Event{
		Author: "agent",
	}

	env := FromFrameworkEvent(ev, FrameworkEventMeta{SessionID: "s"}, EnvelopeTypeTextDelta)

	if env.Actions != nil {
		t.Error("Actions should be nil when framework event has no actions")
	}
}

func TestFromFrameworkEvent_EmptyTimestamp(t *testing.T) {
	ev := &trpcevent.Event{
		Author: "agent",
	}

	env := FromFrameworkEvent(ev, FrameworkEventMeta{SessionID: "s"}, EnvelopeTypeTextDelta)

	// NewEnvelope sets a default timestamp, FromFrameworkEvent should not overwrite
	// when framework timestamp is zero.
	if env.Timestamp == "" {
		t.Error("Timestamp should not be empty (NewEnvelope provides default)")
	}
}

func TestFromFrameworkEvent_WithResponse(t *testing.T) {
	ev := &trpcevent.Event{
		Author: "agent",
	}
	ev.Response = &trpcmodel.Response{
		Object: trpcmodel.ObjectTypeChatCompletionChunk,
	}

	env := FromFrameworkEvent(ev, FrameworkEventMeta{SessionID: "s"}, EnvelopeTypeTextDelta)

	// Response is not extracted by FromFrameworkEvent (handled by specific projectors)
	if env.Content != nil {
		t.Error("Content should be nil (FromFrameworkEvent only extracts common fields)")
	}
}

func TestIsJSONString(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{`"hello"`, true},
		{`"hello world"`, true},
		{`""`, true},
		{`42`, false},
		{`true`, false},
		{`null`, false},
		{`{"key":"val"}`, false},
		{`[1,2,3]`, false},
		{`"`, false},
		{``, false},
	}
	for _, tt := range tests {
		got := isJSONString(json.RawMessage(tt.input))
		if got != tt.want {
			t.Errorf("isJSONString(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestCoalesceStr(t *testing.T) {
	if got := coalesceStr("a", "b"); got != "a" {
		t.Errorf("coalesceStr(a, b) = %q, want %q", got, "a")
	}
	if got := coalesceStr("", "b"); got != "b" {
		t.Errorf("coalesceStr(_, b) = %q, want %q", got, "b")
	}
	if got := coalesceStr("", ""); got != "" {
		t.Errorf("coalesceStr(_, _) = %q, want empty", got)
	}
}

// Ensure FromFrameworkEvent returns contract.Envelope (not pointer).
func TestFromFrameworkEvent_ReturnType(t *testing.T) {
	ev := &trpcevent.Event{Author: "agent"}
	env := FromFrameworkEvent(ev, FrameworkEventMeta{SessionID: "s"}, EnvelopeTypeTextDelta)
	var _ contract.Envelope = env
}
