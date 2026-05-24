package biz

import "context"

// OutboundDeliveryPort is the platform-agnostic interface for delivering
// outbound messages from any channel adapter. Each platform adapter
// implements this to translate a generic delivery request into its
// platform-specific API call.
//
// Implementations live in internal/channel/<platform>. Wire binding
// happens in internal/service.
type OutboundDeliveryPort interface {
	// Deliver sends a message to the specified target.
	Deliver(ctx context.Context, target OutboundTarget, message OutboundMessage) error

	// DeliverCard sends a rich card / interactive message.
	DeliverCard(ctx context.Context, target OutboundTarget, card OutboundCard) error
}

// OutboundTarget identifies the recipient of an outbound message.
type OutboundTarget struct {
	Recipient string
	Meta      map[string]string
}

// OutboundMessage is a plain-text outbound message.
type OutboundMessage struct {
	Content string
}

// OutboundCard is a rich interactive card for platform adapters.
type OutboundCard struct {
	TemplateID string
	Data       map[string]any
	Fallback   string // plain-text fallback for platforms without card support
}
