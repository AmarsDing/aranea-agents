package team

import (
	"encoding/json"
	"strings"
	"testing"
)

func Test_mergeTeamUserTurnMetaJSON_previewAndFlags(t *testing.T) {
	raw, err := mergeTeamUserTurnMetaJSON(`{"dialog_mode":"plan"}`, "Hi", "Hi\nwrapped")
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatal(err)
	}
	if m["dialog_mode"] != "plan" {
		t.Fatalf("preserve existing key: %v", m)
	}
	if m["team_user_send_differs_from_display"] != true {
		t.Fatalf("expected send differs: %v", m)
	}
	wantLen := float64(len([]rune("Hi\nwrapped")))
	if got := m["user_turn_length"]; got != wantLen {
		t.Fatalf("user_turn_length: got %v want %v", got, wantLen)
	}
	prev, ok := m["team_user_send_preview"].(string)
	if !ok || !strings.HasPrefix(prev, "Hi") {
		t.Fatalf("preview: %q", prev)
	}
	prev2, ok2 := m["user_text_preview"].(string)
	if !ok2 || prev2 != prev {
		t.Fatalf("user_text_preview: %q want %q", prev2, prev)
	}
}
