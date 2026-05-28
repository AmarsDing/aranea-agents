package port

import (
	"strings"
)

// Well-known OutboundMeta keys (CH-BOR-10). Adapters must use these names; routing
// fields like thread_id must not be hard-coded outside port/build helpers.
const (
	MetaRecipient         = "recipient"
	MetaReceiveIDType     = "receive_id_type"
	MetaChatID            = "chat_id"
	MetaChatType          = "chat_type"
	MetaThreadID          = "thread_id"
	MetaReplyInThread     = "reply_in_thread"
	MetaSenderOpenID      = "sender_open_id"
	MetaSenderUserID      = "sender_user_id"
	MetaSenderType        = "sender_type"
	MetaMentioned         = "mentioned"
	MetaIngressSource     = "ingress_source"
	MetaInboundMessageID  = "inbound_message_id"
	MetaSessionID         = "session_id"
	MetaSessionWebhook    = "session_webhook"
	MetaResponseURL       = "response_url"
	MetaServiceURL        = "service_url"
	MetaConversationID    = "conversation_id"
	MetaReplyToken        = "reply_token"
)

var knownOutboundMetaKeys = map[string]struct{}{
	MetaRecipient:        {},
	MetaReceiveIDType:    {},
	MetaChatID:           {},
	MetaChatType:         {},
	MetaThreadID:         {},
	MetaReplyInThread:    {},
	MetaSenderOpenID:     {},
	MetaSenderUserID:     {},
	MetaSenderType:       {},
	MetaMentioned:        {},
	MetaIngressSource:    {},
	MetaInboundMessageID: {},
	MetaSessionID:        {},
	MetaSessionWebhook:   {},
	MetaResponseURL:      {},
	MetaServiceURL:       {},
	MetaConversationID:   {},
	MetaReplyToken:       {},
}

var platformRequiredOutboundMeta = map[string][]string{
	"feishu":     {MetaRecipient},
	"lark":       {MetaRecipient},
	"slack":      {MetaResponseURL},
	"teams":      {MetaServiceURL, MetaConversationID},
	"mattermost": {MetaRecipient},
	"line":       {MetaRecipient},
}

// LocalKeyFromMeta returns a stable routing key for preview/run registry (CH-BOR-07/10).
// Prefer chat_id + thread_id; fall back to recipient when thread routing is absent.
func LocalKeyFromMeta(platform string, meta map[string]string) string {
	if len(meta) == 0 {
		return ""
	}
	chatID := strings.TrimSpace(meta[MetaChatID])
	threadID := strings.TrimSpace(meta[MetaThreadID])
	if chatID != "" || threadID != "" {
		if threadID != "" {
			return chatID + ":" + threadID
		}
		return chatID
	}
	return strings.TrimSpace(meta[MetaRecipient])
}

// ValidateOutboundMeta reports non-fatal contract violations for observability and adapter QA.
func ValidateOutboundMeta(platform string, meta map[string]string) []string {
	if meta == nil {
		meta = map[string]string{}
	}
	platform = strings.ToLower(strings.TrimSpace(platform))
	var issues []string
	for key := range meta {
		k := strings.TrimSpace(key)
		if k == "" {
			continue
		}
		if _, ok := knownOutboundMetaKeys[k]; ok {
			continue
		}
		if strings.HasPrefix(k, "x_") {
			continue
		}
		issues = append(issues, "unknown meta key: "+k)
	}
	for _, req := range platformRequiredOutboundMeta[platform] {
		if strings.TrimSpace(meta[req]) == "" {
			issues = append(issues, "missing required meta: "+req)
		}
	}
	return issues
}

// NormalizeOutboundMeta returns a trimmed copy suitable for outbound delivery passthrough.
func NormalizeOutboundMeta(meta map[string]string) map[string]string {
	if len(meta) == 0 {
		return nil
	}
	out := make(map[string]string, len(meta))
	for k, v := range meta {
		key := strings.TrimSpace(k)
		if key == "" {
			continue
		}
		out[key] = strings.TrimSpace(v)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
