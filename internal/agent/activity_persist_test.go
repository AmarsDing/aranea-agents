package agent

import (
	"encoding/json"
	"strings"
	"testing"

	"aranea-agents/internal/biz"
)

func TestActivityMessageID(t *testing.T) {
	id, err := ActivityMessageID("tc-1")
	if err != nil {
		t.Fatal(err)
	}
	if id != "act-tc-1" {
		t.Fatalf("id: %q", id)
	}
	if _, err := ActivityMessageID(""); err == nil {
		t.Fatal("expected error for empty tool_call id")
	}
}

func TestActivityMessageStatus(t *testing.T) {
	tests := map[string]string{
		"calling":   "tool_running",
		"running":   "tool_running",
		"blocked":   "tool_blocked",
		"cancelled": "tool_cancelled",
		"failed":    "tool_failed",
		"success":   "tool_success",
	}
	for in, want := range tests {
		if got := ActivityMessageStatus(in); got != want {
			t.Fatalf("%s: got %q want %q", in, got, want)
		}
	}
}

func TestCancelledActivityMessage(t *testing.T) {
	opts, _ := json.Marshal(map[string]any{
		"schema": ChatActivitySchemaV1,
		"agent":  map[string]string{"name": "Worker", "agent_key": "worker-a"},
		"tool_event": map[string]any{
			"id":            "tc-1",
			"status":        "running",
			"tool_name":     "read_file",
			"display_label": "读取文件",
			"agent_name":    "Worker",
		},
	})
	msg := biz.ChatMessage{
		ID:              "act-tc-1",
		SessionID:       "sess-1",
		Status:          "tool_running",
		OptionsJSON:     string(opts),
		ContentMarkdown: "running",
	}
	next, ok := CancelledActivityMessage(msg)
	if !ok {
		t.Fatal("expected ok")
	}
	if next.Status != "tool_cancelled" {
		t.Fatalf("status=%q", next.Status)
	}
	if !strings.Contains(next.ContentMarkdown, "已取消") {
		t.Fatalf("markdown=%q", next.ContentMarkdown)
	}
}
