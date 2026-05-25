package artifact_test

import (
	"encoding/json"
	"testing"

	"aranea-agents/internal/biz/artifact"
)

func TestMergeRefsIntoOptionsJSON(t *testing.T) {
	merged, err := artifact.MergeRefsIntoOptionsJSON(`{"dialog_mode":"chat"}`, []artifact.Ref{
		{ID: "a1", Name: "out.csv", MimeType: "text/csv", Size: 100},
	})
	if err != nil {
		t.Fatal(err)
	}
	var opts map[string]any
	if err := json.Unmarshal([]byte(merged), &opts); err != nil {
		t.Fatal(err)
	}
	atts, ok := opts["attachments"].([]any)
	if !ok || len(atts) != 1 {
		t.Fatalf("attachments=%v", opts["attachments"])
	}
}
