package port

import (
	"context"

	"aranea-agents/internal/biz"
)

// InboundHandler processes normalized ingress (implemented by service.ChannelIngress; see runtime.Manager).
type InboundHandler interface {
	ProcessInbound(ctx context.Context, ch biz.Channel, ev InboundEvent) error
}

// StreamPreviewUpdater patches one IM preview message during streaming Turns (F-07).
type StreamPreviewUpdater interface {
	UpdatePreview(ctx context.Context, recipient, text string, force bool) error
	PreviewMessageID() string
}
