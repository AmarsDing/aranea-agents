// Package port defines transport-neutral channel message types (MuseBot robot/* adapted).
package port

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
