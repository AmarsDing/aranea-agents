package biz

import (
	"encoding/json"
	"strings"
)

// ResolveChannelWebOrigin returns the public web app origin for deep links (IM tool cards, async links).
// Prefers metadata.web_app_origin, then metadata.public_webhook_origin.
func ResolveChannelWebOrigin(metadataJSON string) string {
	metadataJSON = strings.TrimSpace(metadataJSON)
	if metadataJSON == "" {
		return ""
	}
	var meta struct {
		WebAppOrigin        string `json:"web_app_origin"`
		PublicWebhookOrigin string `json:"public_webhook_origin"`
	}
	if json.Unmarshal([]byte(metadataJSON), &meta) != nil {
		return ""
	}
	if o := normalizeOrigin(meta.WebAppOrigin); o != "" {
		return o
	}
	return normalizeOrigin(meta.PublicWebhookOrigin)
}

func normalizeOrigin(raw string) string {
	return strings.TrimRight(strings.TrimSpace(raw), "/")
}
