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
