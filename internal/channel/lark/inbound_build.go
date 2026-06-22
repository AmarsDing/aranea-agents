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
	meta := map[string]string{
		port.MetaRecipient:        recipient,
		port.MetaReceiveIDType:    receiveType,
		port.MetaChatID:           chatID,
		port.MetaChatType:         chatType,
		port.MetaSenderOpenID:     openID,
		port.MetaSenderUserID:     userID,
		port.MetaSenderType:       strings.TrimSpace(p.SenderType),
		port.MetaMentioned:        boolMeta(p.Mentioned),
		port.MetaIngressSource:    strings.TrimSpace(p.IngressSource),
		port.MetaInboundMessageID: msgID,
	}
	if tid := strings.TrimSpace(p.ThreadID); tid != "" {
		meta[port.MetaThreadID] = tid
	}
	return port.InboundEvent{
		PlatformType:   "feishu",
		PeerID:         port.FirstNonEmpty(openID, userID, chatID),
		Text:           strings.TrimSpace(p.Text),
		IdempotencyKey: "feishu:" + msgID,
		OutboundMeta:   meta,
	}, true, ""
}
