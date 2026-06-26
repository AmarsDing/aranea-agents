package biz

import (
	"encoding/json"
	"testing"
	"time"
)

func TestActivityJSONSerializationKeys(t *testing.T) {
	a := Activity{
		ID:               "act-123",
		Kind:             ActivityKindTask,
		Status:           ActivityStatusRunning,
		SessionID:        "sess-456",
		TurnID:           "turn-789",
		ParentActivityID: "",
		Timestamp:        time.Date(2026, 6, 27, 10, 0, 0, 0, time.UTC),
		DurationMs:       1500,
		Seq:              42,
		PromptTokens:     100,
		CompletionTokens: 200,
		Content:          "hello world",
		AgentKey:         "default",
		AgentName:        "Default Agent",
		Collapsed:        false,
		Meta:             map[string]any{"foo": "bar"},
	}
	data, err := json.Marshal(a)
	if err != nil {
		t.Fatalf("marshal activity: %v", err)
	}
	t.Logf("serialized keys: %s", string(data))

	// Frontend activityEvent.ts expects snake_case keys.
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal to map: %v", err)
	}
	if _, ok := raw["content"]; !ok {
		t.Errorf("missing snake_case key 'content'; got keys %v", raw)
	}
	if _, ok := raw["session_id"]; !ok {
		t.Errorf("missing snake_case key 'session_id'; got keys %v", raw)
	}
	// parent_activity_id is omitted when empty (omitempty). For non-root
	// activities it must be serialized; verify with a non-empty value below.
	// Make sure we do NOT emit Go-default PascalCase keys.
	if _, ok := raw["Content"]; ok {
		t.Errorf("unexpected PascalCase key 'Content' emitted")
	}
	if _, ok := raw["SessionID"]; ok {
		t.Errorf("unexpected PascalCase key 'SessionID' emitted")
	}
}
