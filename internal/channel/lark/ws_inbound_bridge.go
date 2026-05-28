package lark

import (
	"context"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/channel/port"
	"aranea-agents/internal/channel/runtime"
)

type wsInboundBridge struct {
	ctx     context.Context
	ch      biz.Channel
	creds   []biz.ChannelCredential
	lookup  runtime.CredentialLookup
	handler port.InboundHandler
}

func (b wsInboundBridge) ProcessInbound(_ context.Context, ch biz.Channel, ev port.InboundEvent) error {
	HandleWSInbound(b.ctx, ch, b.creds, b.lookup, b.handler, ev)
	return nil
}
