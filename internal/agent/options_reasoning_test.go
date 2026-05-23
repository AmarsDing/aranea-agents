package agent

import (
	"encoding/json"
	"testing"
)

func TestMergeReasoningIntoAssistantOptionsJSON_dualKeys(t *testing.T) {
	out, err := MergeReasoningIntoAssistantOptionsJSON(`{"dialog_mode":"plan"}`, "step one")
	if err != nil {
		t.Fatal(err)
	}
	var opts map[string]any
	if err := json.Unmarshal([]byte(out), &opts); err != nil {
		t.Fatal(err)
	}
	if opts["reasoning_markdown"] != "step one" || opts["reasoning_content"] != "step one" {
		t.Fatalf("opts = %#v", opts)
	}
	cleared, err := MergeReasoningIntoAssistantOptionsJSON(out, "")
	if err != nil {
		t.Fatal(err)
	}
	var clearedOpts map[string]any
	if err := json.Unmarshal([]byte(cleared), &clearedOpts); err != nil {
		t.Fatal(err)
	}
	if _, ok := clearedOpts["reasoning_markdown"]; ok {
		t.Fatal("expected reasoning_markdown cleared")
	}
	if _, ok := clearedOpts["reasoning_content"]; ok {
		t.Fatal("expected reasoning_content cleared")
	}
}
