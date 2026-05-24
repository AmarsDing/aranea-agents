package lark

import (
	"encoding/json"
	"strings"

	"aranea-agents/internal/channel/port"

	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
)

type messageText struct {
	Text string `json:"text"`
}

// stripFeishuMentions removes @mention keys from plain text bodies.
func stripFeishuMentions(text string, mentions []*larkim.MentionEvent) string {
	text = strings.TrimSpace(text)
	for _, mention := range mentions {
		if mention == nil || mention.Key == nil {
			continue
		}
		key := strings.TrimSpace(*mention.Key)
		if key == "" {
			continue
		}
		text = strings.ReplaceAll(text, key, "")
	}
	return strings.TrimSpace(text)
}

func feishuChatType(message *larkim.P2MessageReceiveV1) string {
	if message == nil || message.Event == nil || message.Event.Message == nil || message.Event.Message.ChatType == nil {
		return ""
	}
	return strings.TrimSpace(*message.Event.Message.ChatType)
}

func feishuSenderType(message *larkim.P2MessageReceiveV1) string {
	if message == nil || message.Event == nil || message.Event.Sender == nil || message.Event.Sender.SenderType == nil {
		return ""
	}
	return strings.TrimSpace(*message.Event.Sender.SenderType)
}

func parseFeishuTextBody(content string, mentions []*larkim.MentionEvent) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return ""
	}
	var body messageText
	if err := json.Unmarshal([]byte(content), &body); err != nil {
		return ""
	}
	return stripFeishuMentions(body.Text, mentions)
}

// ParseFeishuPostBody converts Feishu post message JSON to plain text (F-05).
func ParseFeishuPostBody(content string) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return ""
	}
	var wrap struct {
		Post map[string]struct {
			Title   string `json:"title"`
			Content [][]struct {
				Tag      string `json:"tag"`
				Text     string `json:"text"`
				Href     string `json:"href"`
				UserID   string `json:"user_id"`
				UserName string `json:"user_name"`
			} `json:"content"`
		} `json:"post"`
	}
	if err := json.Unmarshal([]byte(content), &wrap); err != nil || len(wrap.Post) == 0 {
		return ""
	}
	var parts []string
	for _, body := range wrap.Post {
		if t := strings.TrimSpace(body.Title); t != "" {
			parts = append(parts, t)
		}
		for _, row := range body.Content {
			var line []string
			for _, inline := range row {
				switch strings.ToLower(strings.TrimSpace(inline.Tag)) {
				case "text", "a":
					if s := strings.TrimSpace(inline.Text); s != "" {
						line = append(line, s)
					} else if s := strings.TrimSpace(inline.Href); s != "" {
						line = append(line, s)
					}
				case "at":
					if s := strings.TrimSpace(inline.UserName); s != "" {
						line = append(line, "@"+s)
					} else if s := strings.TrimSpace(inline.UserID); s != "" {
						line = append(line, "@"+s)
					}
				default:
					if s := strings.TrimSpace(inline.Text); s != "" {
						line = append(line, s)
					}
				}
			}
			if len(line) > 0 {
				parts = append(parts, strings.Join(line, ""))
			}
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

func parseFeishuMessageBody(content, msgType string, mentions []*larkim.MentionEvent) string {
	switch strings.TrimSpace(strings.ToLower(msgType)) {
	case "post":
		if text := ParseFeishuPostBody(content); text != "" {
			return text
		}
	}
	return parseFeishuTextBody(content, mentions)
}

// InboundEventFromWSMessage normalizes larkws P2MessageReceiveV1 into a channel inbound event.
func InboundEventFromWSMessage(message *larkim.P2MessageReceiveV1) (port.InboundEvent, bool) {
	if message == nil || message.Event == nil || message.Event.Message == nil {
		return port.InboundEvent{}, false
	}
	msg := message.Event.Message
	content := ""
	if msg.Content != nil {
		content = *msg.Content
	}
	openID := ""
	userID := ""
	if message.Event.Sender != nil && message.Event.Sender.SenderId != nil {
		if message.Event.Sender.SenderId.OpenId != nil {
			openID = strings.TrimSpace(*message.Event.Sender.SenderId.OpenId)
		}
		if message.Event.Sender.SenderId.UserId != nil {
			userID = strings.TrimSpace(*message.Event.Sender.SenderId.UserId)
		}
	}
	chatID := ""
	if msg.ChatId != nil {
		chatID = strings.TrimSpace(*msg.ChatId)
	}
	msgID := ""
	if msg.MessageId != nil {
		msgID = strings.TrimSpace(*msg.MessageId)
	}
	msgType := ""
	if msg.MessageType != nil {
		msgType = strings.TrimSpace(*msg.MessageType)
	}
	ev, ok, _ := BuildFeishuInboundEvent(FeishuInboundParams{
		MessageID:     msgID,
		ChatID:        chatID,
		ChatType:      feishuChatType(message),
		SenderType:    feishuSenderType(message),
		MessageType:   msgType,
		Text:          parseFeishuMessageBody(content, msgType, msg.Mentions),
		Mentioned:     len(msg.Mentions) > 0,
		OpenID:        openID,
		UserID:        userID,
		ThreadID:      feishuMessageThreadID(msg),
		IngressSource: "websocket",
	})
	return ev, ok
}

func boolMeta(v bool) string {
	if v {
		return "true"
	}
	return "false"
}

// InferChatTypeFromChatID best-effort chat type when only chat_id is available (webhook path).
func InferChatTypeFromChatID(chatID string) string {
	chatID = strings.TrimSpace(chatID)
	if strings.HasPrefix(chatID, "oc_") {
		return "group"
	}
	if chatID != "" {
		return "p2p"
	}
	return ""
}

func firstNonEmptyPeerID(parts ...string) string {
	for _, p := range parts {
		if strings.TrimSpace(p) != "" {
			return strings.TrimSpace(p)
		}
	}
	return ""
}

func feishuMessageThreadID(msg *larkim.EventMessage) string {
	if msg == nil || msg.ThreadId == nil {
		return ""
	}
	return strings.TrimSpace(*msg.ThreadId)
}
