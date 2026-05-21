package data

import (
	"encoding/json"
	"testing"

	"aranea-agents/internal/biz"
)

func TestRunStatusFromStateJSON(t *testing.T) {
	raw, _ := json.Marshal(map[string]string{biz.SessionStateRunStatus: "failed"})
	if got := runStatusFromStateJSON(string(raw)); got != "failed" {
		t.Fatalf("got %q want failed", got)
	}
	if runStatusFromStateJSON("{}") != "" {
		t.Fatal("empty object should yield empty status")
	}
}
