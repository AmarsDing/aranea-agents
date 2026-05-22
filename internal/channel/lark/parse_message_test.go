package lark

import (
	"testing"

	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
)

func TestStripFeishuMentions(t *testing.T) {
	got := stripFeishuMentions("@_user_1 你好", nil)
	if got != "@_user_1 你好" {
		t.Fatalf("unexpected %q", got)
	}
	key := "@_user_1"
	got = stripFeishuMentions("@_user_1 你好", []*larkim.MentionEvent{{Key: &key}})
	if got != "你好" {
		t.Fatalf("unexpected %q", got)
	}
}

func TestParseFeishuTextBody(t *testing.T) {
	got := parseFeishuTextBody(`{"text":"@_user_1 ping"}`, nil)
	if got != "@_user_1 ping" {
		t.Fatalf("unexpected %q", got)
	}
	key := "@_user_1"
	got = parseFeishuTextBody(`{"text":"@_user_1 ping"}`, []*larkim.MentionEvent{{Key: &key}})
	if got != "ping" {
		t.Fatalf("unexpected %q", got)
	}
}

func TestShouldProcessFeishuMessageGroupRequiresMention(t *testing.T) {
	group := "group"
	p2p := "p2p"
	msgType := "text"
	content := `{"text":"hi"}`
	groupNoMention := &larkim.P2MessageReceiveV1{
		Event: &larkim.P2MessageReceiveV1Data{
			Message: &larkim.EventMessage{
				ChatType:    &group,
				MessageType: &msgType,
				Content:     &content,
			},
		},
	}
	if shouldProcessFeishuMessage(groupNoMention) {
		t.Fatal("expected group without mention to be skipped")
	}
	key := "@_user_1"
	groupMention := &larkim.P2MessageReceiveV1{
		Event: &larkim.P2MessageReceiveV1Data{
			Message: &larkim.EventMessage{
				ChatType:    &group,
				MessageType: &msgType,
				Content:     &content,
				Mentions:    []*larkim.MentionEvent{{Key: &key}},
			},
		},
	}
	if !shouldProcessFeishuMessage(groupMention) {
		t.Fatal("expected group with mention to pass")
	}
	p2pMsg := &larkim.P2MessageReceiveV1{
		Event: &larkim.P2MessageReceiveV1Data{
			Message: &larkim.EventMessage{
				ChatType:    &p2p,
				MessageType: &msgType,
				Content:     &content,
			},
		},
	}
	if !shouldProcessFeishuMessage(p2pMsg) {
		t.Fatal("expected p2p to pass")
	}
}

func TestInboundEventFromWSMessageUsesChatID(t *testing.T) {
	p2p := "p2p"
	msgType := "text"
	content := `{"text":"hello"}`
	chatID := "oc_group_1"
	openID := "ou_user_1"
	ev, ok := InboundEventFromWSMessage(&larkim.P2MessageReceiveV1{
		Event: &larkim.P2MessageReceiveV1Data{
			Sender: &larkim.EventSender{
				SenderId: &larkim.UserId{OpenId: &openID},
			},
			Message: &larkim.EventMessage{
				ChatType:    &p2p,
				MessageType: &msgType,
				Content:     &content,
				ChatId:      &chatID,
			},
		},
	})
	if !ok {
		t.Fatal("expected event")
	}
	if ev.OutboundMeta["recipient"] != chatID {
		t.Fatalf("recipient=%q want chat_id", ev.OutboundMeta["recipient"])
	}
	if ev.OutboundMeta["receive_id_type"] != ReceiveIDTypeChatID {
		t.Fatalf("receive_id_type=%q", ev.OutboundMeta["receive_id_type"])
	}
}
