package lark

import (
	"strings"

	"aranea-agents/internal/channel/port"
)

// BuildFeishuInboundEvent applies AcceptFeishuInbound and builds a normalized port.InboundEvent.
func BuildFeishuInboundEvent(p FeishuInboundParams) (port.InboundEvent, bool, string) {
	ok, reason := AcceptFeishuInbound(p)
	if !ok {
		return port.InboundEvent{}, false, reason
	}
	openID := strings.TrimSpace(p.OpenID)
	userID := strings.TrimSpace(p.UserID)
	chatID := strings.TrimSpace(p.ChatID)
	msgID := strings.TrimSpace(p.MessageID)
	chatType := strings.TrimSpace(p.ChatType)
	if chatType == "" && chatID != "" {
		chatType = InferChatTypeFromChatID(chatID)
	}
	recipient, receiveType := ResolveReceiveTarget(openID, userID, chatID)
	return port.InboundEvent{
		PlatformType:   "feishu",
		PeerID:         firstNonEmptyPeerID(openID, userID, chatID),
		Text:           strings.TrimSpace(p.Text),
		IdempotencyKey: "feishu:" + msgID,
		OutboundMeta: map[string]string{
			"recipient":       recipient,
			"receive_id_type": receiveType,
			"chat_id":         chatID,
			"chat_type":       chatType,
			"sender_open_id":  openID,
			"sender_user_id":  userID,
			"sender_type":     strings.TrimSpace(p.SenderType),
			"mentioned":       boolMeta(p.Mentioned),
			"ingress_source":  strings.TrimSpace(p.IngressSource),
		},
	}, true, ""
}
