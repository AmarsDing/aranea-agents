package data

import (
	"encoding/json"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

func TestRunStatusFromStateJSON(t *testing.T) {
	lg := loggateway.NewNoop()
	raw, _ := json.Marshal(map[string]string{biz.SessionStateRunStatus: "failed"})
	if got := runStatusFromStateJSON(lg, string(raw)); got != "failed" {
		t.Fatalf("got %q want failed", got)
	}
	if runStatusFromStateJSON(lg, "{}") != "" {
		t.Fatal("empty object should yield empty status")
	}
}
