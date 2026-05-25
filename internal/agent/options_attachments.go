package agent

import (
	"encoding/json"
	"strings"
)

// MessageAttachmentRef is persisted in user message options_json for chat bubble replay (ART-01).
type MessageAttachmentRef struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	MimeType string `json:"mime_type"`
	Size     int64  `json:"size,omitempty"`
}

// MergeAttachmentsIntoUserOptionsJSON adds attachment metadata for inline chat replay.
func MergeAttachmentsIntoUserOptionsJSON(optionsJSON string, attachments []MessageAttachmentRef) (string, error) {
	if len(attachments) == 0 {
		return optionsJSON, nil
	}
	opts := map[string]any{}
	if raw := strings.TrimSpace(optionsJSON); raw != "" {
		if err := json.Unmarshal([]byte(raw), &opts); err != nil {
			return optionsJSON, err
		}
	}
	refs := make([]map[string]any, 0, len(attachments))
	for _, a := range attachments {
		if strings.TrimSpace(a.ID) == "" {
			continue
		}
		ref := map[string]any{
			"id":        a.ID,
			"name":      a.Name,
			"mime_type": a.MimeType,
		}
		if a.Size > 0 {
			ref["size"] = a.Size
		}
		refs = append(refs, ref)
	}
	if len(refs) == 0 {
		return optionsJSON, nil
	}
	opts["attachments"] = refs
	out, err := json.Marshal(opts)
	if err != nil {
		return "", err
	}
	return string(out), nil
}
