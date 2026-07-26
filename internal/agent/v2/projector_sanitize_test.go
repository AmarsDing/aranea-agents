package v2

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"aranea-agents/internal/biz"
)

// 2026-07-25 22:33 incident: malformed LLM tool arguments were stored raw
// into Step.ToolArgs (json.RawMessage). MarshalJSON validates RawMessage, so
// one bad tool call dropped the whole step.updated/step.completed event from
// BOTH the outbox persist path and the WS subscriber (logs:
// "outbox marshal failed" / "ws v2 marshal failed"). sanitizeRawJSON demotes
// invalid JSON to a plain JSON string so events always stay marshalable.

func TestSanitizeRawJSON_validPassthrough(t *testing.T) {
	in := []byte(`{"a":1,"b":"x"}`)
	out := sanitizeRawJSON(in)
	if string(out) != string(in) {
		t.Fatalf("valid JSON must pass through unchanged, got: %s", out)
	}
}

func TestSanitizeRawJSON_invalidDemotedToString(t *testing.T) {
	in := []byte(`{"a":1}}`)
	out := sanitizeRawJSON(in)
	if !json.Valid(out) {
		t.Fatalf("sanitized output must be valid JSON, got: %s", out)
	}
	var s string
	if err := json.Unmarshal(out, &s); err != nil {
		t.Fatalf("sanitized output must decode as JSON string: %v", err)
	}
	if s != string(in) {
		t.Fatalf("original content must be preserved, got: %q", s)
	}
}

func TestSanitizeRawJSON_emptyAndNil(t *testing.T) {
	if out := sanitizeRawJSON(nil); out != nil {
		t.Fatalf("nil input must stay nil, got: %s", out)
	}
	if out := sanitizeRawJSON([]byte{}); out != nil {
		t.Fatalf("empty input must stay nil, got: %s", out)
	}
}

func TestOnToolCall_malformedArgs_eventStillMarshalable(t *testing.T) {
	p, capture := testProjector()
	capture.events = nil

	malformed := json.RawMessage(`{"data":{"summary":"s"},"topic":"t"}}`)
	p.OnToolCall(context.Background(), ProjectMeta{
		TaskID: "task-1", TurnID: "turn-1", SessionID: "sess-1",
		SpiritSessionID: "spirit-1", AgentKey: "agent-1",
	}, "set_deliverable", malformed)

	if len(capture.events) == 0 {
		t.Fatal("expected step events")
	}
	for i, ev := range capture.events {
		se, ok := ev.(*biz.StepUpdatedEvent)
		if !ok {
			continue
		}
		if _, err := json.Marshal(se.Step); err != nil {
			t.Fatalf("event %d: Step must marshal cleanly (was dropped before fix): %v", i, err)
		}
		if !json.Valid(se.Step.ToolArgs) {
			t.Fatalf("event %d: ToolArgs must be valid JSON after sanitize, got: %s", i, se.Step.ToolArgs)
		}
		var s string
		if err := json.Unmarshal(se.Step.ToolArgs, &s); err != nil || !strings.Contains(s, `"topic"`) {
			t.Fatalf("event %d: original args text must be preserved as string: %v %q", i, err, s)
		}
	}
}

func TestCompleteStep_malformedToolResult_eventStillMarshalable(t *testing.T) {
	p, capture := testProjector()
	capture.events = nil

	meta := ProjectMeta{
		TaskID: "task-1", TurnID: "turn-1", SessionID: "sess-1",
		SpiritSessionID: "spirit-1", AgentKey: "agent-1",
	}
	stepID := p.OnToolCall(context.Background(), meta, "read_file", json.RawMessage(`{"path":"x"}`))
	capture.events = nil
	if stepID == "" {
		t.Fatal("expected stepID from OnToolCall")
	}
	// Tool returned non-JSON text content (e.g. raw file bytes / error blob).
	p.OnToolResult(context.Background(), stepID, json.RawMessage("raw bytes \x00 not json"), nil)

	for i, ev := range capture.events {
		ce, ok := ev.(*biz.StepCompletedEvent)
		if !ok {
			continue
		}
		if _, err := json.Marshal(ce.Step); err != nil {
			t.Fatalf("event %d: Step must marshal cleanly: %v", i, err)
		}
		if !json.Valid(ce.Step.ToolResult) {
			t.Fatalf("event %d: ToolResult must be valid JSON after sanitize, got: %s", i, ce.Step.ToolResult)
		}
	}
}

func TestEmitConfirmRequest_malformedArgs_eventStillMarshalable(t *testing.T) {
	p, capture := testProjector()
	capture.events = nil

	_, err := p.EmitConfirmRequest(context.Background(), biz.ActivityConfirmParams{
		ToolName:      "exec_command",
		ToolArguments: `{"cmd":"ls"}}`,
		Content:       "confirm?",
	})
	if err != nil {
		t.Fatalf("EmitConfirmRequest: %v", err)
	}
	for i, ev := range capture.events {
		ue, ok := ev.(*biz.StepUpdatedEvent)
		if !ok {
			continue
		}
		if _, err := json.Marshal(ue.Step); err != nil {
			t.Fatalf("event %d: Step must marshal cleanly: %v", i, err)
		}
		if !json.Valid(ue.Step.ToolArgs) {
			t.Fatalf("event %d: ToolArgs must be valid JSON after sanitize, got: %s", i, ue.Step.ToolArgs)
		}
	}
}
