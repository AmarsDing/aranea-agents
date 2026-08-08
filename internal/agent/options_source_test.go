package agent

import (
	"encoding/json"
	"testing"
)

func TestMergeSourceIntoUserOptionsJSON(t *testing.T) {
	merged, err := MergeSourceIntoUserOptionsJSON(`{"dialog_mode":"chat"}`, "channel")
	if err != nil {
		t.Fatal(err)
	}
	var opts map[string]any
	if err := json.Unmarshal([]byte(merged), &opts); err != nil {
		t.Fatal(err)
	}
	if opts["source"] != "channel" {
		t.Fatalf("source=%v", opts["source"])
	}
	unchanged, err := MergeSourceIntoUserOptionsJSON(merged, "")
	if err != nil || unchanged != merged {
		t.Fatalf("empty source should be no-op: err=%v", err)
	}
}

func TestMergeInboundSourceIntoUserOptionsJSON_platform(t *testing.T) {
	merged, err := MergeInboundSourceIntoUserOptionsJSON(`{}`, "channel", "feishu", "ops-bot")
	if err != nil {
		t.Fatal(err)
	}
	var opts map[string]any
	if err := json.Unmarshal([]byte(merged), &opts); err != nil {
		t.Fatal(err)
	}
	if opts["source"] != "channel" || opts["platform"] != "feishu" || opts["channel_key"] != "ops-bot" {
		t.Fatalf("opts=%v", opts)
	}
}

func TestMergeVoiceMetaIntoUserOptionsJSON(t *testing.T) {
	merged, err := MergeVoiceMetaIntoUserOptionsJSON(`{"dialog_mode":"chat"}`, "volcengine", 1200)
	if err != nil {
		t.Fatal(err)
	}
	var opts map[string]any
	if err := json.Unmarshal([]byte(merged), &opts); err != nil {
		t.Fatal(err)
	}
	if opts["input_modality"] != "voice" {
		t.Fatalf("input_modality=%v", opts["input_modality"])
	}
	if opts["asr_provider"] != "volcengine" {
		t.Fatalf("asr_provider=%v", opts["asr_provider"])
	}
	if opts["asr_duration_ms"] != float64(1200) {
		t.Fatalf("asr_duration_ms=%v", opts["asr_duration_ms"])
	}
	if opts["dialog_mode"] != "chat" {
		t.Fatalf("existing keys must be preserved: %v", opts)
	}

	// 空 provider / 零时长不落键
	merged, err = MergeVoiceMetaIntoUserOptionsJSON(`{}`, "  ", 0)
	if err != nil {
		t.Fatal(err)
	}
	opts = map[string]any{}
	if err := json.Unmarshal([]byte(merged), &opts); err != nil {
		t.Fatal(err)
	}
	if _, ok := opts["asr_provider"]; ok {
		t.Fatalf("blank provider must be omitted: %v", opts)
	}
	if _, ok := opts["asr_duration_ms"]; ok {
		t.Fatalf("zero duration must be omitted: %v", opts)
	}
	if opts["input_modality"] != "voice" {
		t.Fatalf("input_modality=%v", opts["input_modality"])
	}
}
