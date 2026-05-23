package preview

import (
	"encoding/json"
	"testing"
)

func TestBuildFeishuEscalateCardJSON_cardV2Callback(t *testing.T) {
	raw, err := BuildFeishuEscalateCardJSON("sr-1", "sess-1", "https://app.test/s/1")
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid([]byte(raw)) {
		t.Fatal("invalid json")
	}
	var card map[string]any
	if err := json.Unmarshal([]byte(raw), &card); err != nil {
		t.Fatal(err)
	}
	if card["schema"] != "2.0" {
		t.Fatalf("schema=%v want 2.0", card["schema"])
	}
	body, ok := card["body"].(map[string]any)
	if !ok {
		t.Fatal("missing body")
	}
	elements := body["elements"].([]any)
	found := false
	for _, el := range elements {
		block, _ := el.(map[string]any)
		if block["tag"] != "column_set" {
			continue
		}
		columns := block["columns"].([]any)
		btn := columns[0].(map[string]any)["elements"].([]any)[0].(map[string]any)
		behaviors := btn["behaviors"].([]any)
		b := behaviors[0].(map[string]any)
		if b["type"] == "callback" {
			val := b["value"].(map[string]any)
			if val["action"] == "background" && val["session_run_id"] == "sr-1" {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("expected v2 callback button behavior")
	}
}
