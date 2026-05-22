package lark

import "strings"

// Feishu inbound reject reasons (logged on skip; not shown in chat).
const (
	RejectMissingMessageID = "missing_message_id"
	RejectNonUserSender    = "non_user_sender"
	RejectUnknownSender    = "unknown_sender_type"
	RejectNonTextMessage   = "non_text_message"
	RejectEmptyText        = "empty_text"
	RejectGroupNoMention   = "group_without_mention"
)

// FeishuInboundParams is the transport-neutral input for one im.message.receive_v1.
type FeishuInboundParams struct {
	MessageID     string
	ChatID        string
	ChatType      string
	SenderType    string
	MessageType   string
	Text          string
	Mentioned     bool
	OpenID        string
	UserID        string
	IngressSource string // "websocket" | "webhook" — for audit only
}

// AcceptFeishuInbound is the single gate for WS and Webhook — only explicit user text with message_id.
func AcceptFeishuInbound(p FeishuInboundParams) (bool, string) {
	if strings.TrimSpace(p.MessageID) == "" {
		return false, RejectMissingMessageID
	}
	if !isFeishuUserSenderType(p.SenderType) {
		if strings.TrimSpace(p.SenderType) == "" {
			return false, RejectUnknownSender
		}
		return false, RejectNonUserSender
	}
	if mt := strings.TrimSpace(p.MessageType); mt != "" && mt != "text" {
		return false, RejectNonTextMessage
	}
	if strings.TrimSpace(p.Text) == "" {
		return false, RejectEmptyText
	}
	if strings.EqualFold(strings.TrimSpace(p.ChatType), "group") && !p.Mentioned {
		return false, RejectGroupNoMention
	}
	return true, ""
}

// isFeishuUserSenderType only accepts explicit user senders (no default-allow on missing type).
func isFeishuUserSenderType(senderType string) bool {
	return strings.EqualFold(strings.TrimSpace(senderType), "user")
}
