package lark

import (
	"strings"

	"aranea-agents/internal/channel/port"
)

// InboundEventFromWebhook applies the same gate as WebSocket for im.message.receive_v1.
func InboundEventFromWebhook(parsed *WebhookParseResult) (port.InboundEvent, bool, string) {
	if parsed == nil {
		return port.InboundEvent{}, false, RejectEmptyText
	}
	return BuildFeishuInboundEvent(FeishuInboundParams{
		MessageID:     parsed.MessageID,
		ChatID:        parsed.ChatID,
		ChatType:      parsed.ChatType,
		SenderType:    parsed.SenderType,
		MessageType:   parsed.MessageType,
		Text:          strings.TrimSpace(parsed.Text),
		Mentioned:     parsed.Mentioned,
		OpenID:        parsed.SenderOpenID,
		UserID:        parsed.SenderUserID,
		IngressSource: "webhook",
	})
}
