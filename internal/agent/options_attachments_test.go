package agent

import (
	"encoding/json"
	"testing"
)

func TestMergeAttachmentsIntoUserOptionsJSON(t *testing.T) {
	merged, err := MergeAttachmentsIntoUserOptionsJSON(`{"dialog_mode":"chat"}`, []MessageAttachmentRef{
		{ID: "a1", Name: "photo.png", MimeType: "image/png", Size: 1024},
	})
	if err != nil {
		t.Fatal(err)
	}
	var opts map[string]any
	if err := json.Unmarshal([]byte(merged), &opts); err != nil {
		t.Fatal(err)
	}
	if opts["dialog_mode"] != "chat" {
		t.Fatalf("dialog_mode=%v", opts["dialog_mode"])
	}
	atts, ok := opts["attachments"].([]any)
	if !ok || len(atts) != 1 {
		t.Fatalf("attachments=%v ok=%v", opts["attachments"], ok)
	}
	first, ok := atts[0].(map[string]any)
	if !ok || first["id"] != "a1" || first["name"] != "photo.png" {
		t.Fatalf("first attachment=%v", atts[0])
	}

	unchanged, err := MergeAttachmentsIntoUserOptionsJSON(merged, nil)
	if err != nil || unchanged != merged {
		t.Fatalf("empty merge should be no-op: %q err=%v", unchanged, err)
	}
}
