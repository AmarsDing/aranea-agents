// Package port defines transport-neutral channel message types (MuseBot robot/* adapted).
package port

import (
	"aranea-agents/pkg/apierror"
)

// ErrCredentialsNotConfigured is returned when a webhook verification is attempted
// but the required credentials (token, secret, key, etc.) are not configured.
var ErrCredentialsNotConfigured = apierror.BadRequest("CHANNEL_CREDENTIAL", "webhook: credentials not configured")

// WebhookTimestampToleranceSec is the maximum allowed clock skew (in seconds)
// between the webhook's timestamp and the server's current time.
// Used by all platform adapters for replay-attack prevention.
const WebhookTimestampToleranceSec int64 = 300 // 5 minutes

// InboundEvent is normalized ingress from any platform adapter.
type InboundEvent struct {
	PlatformType   string
	PeerID         string
	PeerKey        string
	Text           string
	IdempotencyKey string
	// OutboundMeta carries platform-specific reply targets (session_webhook, response_url, chat_id, …).
	OutboundMeta map[string]string
}

// OutboundTarget resolves where to send a reply for a prior inbound event.
type OutboundTarget struct {
	Recipient string
	Meta      map[string]string
}
