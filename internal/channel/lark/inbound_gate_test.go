package lark

import "testing"

func TestAcceptFeishuInbound(t *testing.T) {
	base := FeishuInboundParams{
		MessageID:   "om_1",
		ChatType:    "p2p",
		SenderType:  "user",
		MessageType: "text",
		Text:        "你好",
	}
	if ok, _ := AcceptFeishuInbound(base); !ok {
		t.Fatal("expected p2p user text accepted")
	}
	if ok, reason := AcceptFeishuInbound(FeishuInboundParams{SenderType: "user", Text: "x"}); ok || reason != RejectMissingMessageID {
		t.Fatalf("missing message_id: ok=%v reason=%q", ok, reason)
	}
	if ok, reason := AcceptFeishuInbound(FeishuInboundParams{MessageID: "om_1", SenderType: "app", Text: "x"}); ok || reason != RejectNonUserSender {
		t.Fatalf("app sender: ok=%v reason=%q", ok, reason)
	}
	if ok, reason := AcceptFeishuInbound(FeishuInboundParams{MessageID: "om_1", SenderType: "", Text: "x"}); ok || reason != RejectUnknownSender {
		t.Fatalf("unknown sender: ok=%v reason=%q", ok, reason)
	}
	group := base
	group.ChatType = "group"
	group.Mentioned = false
	if ok, reason := AcceptFeishuInbound(group); ok || reason != RejectGroupNoMention {
		t.Fatalf("group no mention: ok=%v reason=%q", ok, reason)
	}
	group.Mentioned = true
	if ok, _ := AcceptFeishuInbound(group); !ok {
		t.Fatal("expected group with mention accepted")
	}
}
