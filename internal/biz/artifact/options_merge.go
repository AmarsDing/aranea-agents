package artifact

import (
	"encoding/json"
	"strings"
)

// MergeRefsIntoOptionsJSON adds attachment refs under options_json.attachments.
func MergeRefsIntoOptionsJSON(optionsJSON string, refs []Ref) (string, error) {
	if len(refs) == 0 {
		return optionsJSON, nil
	}
	opts := map[string]any{}
	if raw := strings.TrimSpace(optionsJSON); raw != "" {
		if err := json.Unmarshal([]byte(raw), &opts); err != nil {
			return optionsJSON, err
		}
	}
	outRefs := make([]map[string]any, 0, len(refs))
	for _, a := range refs {
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
		outRefs = append(outRefs, ref)
	}
	if len(outRefs) == 0 {
		return optionsJSON, nil
	}
	opts["attachments"] = outRefs
	raw, err := json.Marshal(opts)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}
