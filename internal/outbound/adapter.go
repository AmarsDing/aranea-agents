package outbound

import (
	"context"

	ch "aranea-agents/internal/channel"
)

type TextSender interface {
	ID() string
	Run(ctx context.Context) error
	SendText(ctx context.Context, target string, text string) error
}

type MessageSender interface {
	ID() string
	Run(ctx context.Context) error
	SendMessage(ctx context.Context, target string, msg OutboundMessage) error
}

type outboundTextAdapter struct {
	inner ch.OutboundText
}

func WrapOutboundText(inner ch.OutboundText) TextSender {
	return &outboundTextAdapter{inner: inner}
}

func (a *outboundTextAdapter) ID() string { return a.inner.ID() }

func (a *outboundTextAdapter) Run(_ context.Context) error { return nil }

func (a *outboundTextAdapter) SendText(ctx context.Context, target string, text string) error {
	return a.inner.SendText(ctx, target, text)
}
